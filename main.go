package main

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path"
	"strings"
	"time"

	sh "github.com/codeskyblue/go-sh"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

type Testsuite struct {
	XMLName    xml.Name `xml:"testsuite"`
	Text       string   `xml:",chardata"`
	Name       string   `xml:"name,attr"`
	Tests      string   `xml:"tests,attr"`
	Failures   string   `xml:"failures,attr"`
	Errors     string   `xml:"errors,attr"`
	Skipped    string   `xml:"skipped,attr"`
	Time       string   `xml:"time,attr"`
	Timestamp  string   `xml:"timestamp,attr"`
	Hostname   string   `xml:"hostname,attr"`
	Properties struct {
		Text     string `xml:",chardata"`
		Property []struct {
			Text  string `xml:",chardata"`
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"property"`
	} `xml:"properties"`
	Testcase []struct {
		Text      string `xml:",chardata"`
		Classname string `xml:"classname,attr"`
		Name      string `xml:"name,attr"`
		Time      string `xml:"time,attr"`
		Failure   struct {
			Text string `xml:",chardata"`
			Type string `xml:"type,attr"`
		} `xml:"failure"`
		Skipped string `xml:"skipped"`
	} `xml:"testcase"`
	SystemOut string `xml:"system-out"`
	SystemErr string `xml:"system-err"`
}

var logger = newLogger()

func newLogger() *logrus.Logger {
	l := logrus.New()
	l.Level = logrus.InfoLevel
	l.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:    true,
		QuoteEmptyFields: true,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "@timestamp",
			logrus.FieldKeyLevel: "@level",
			logrus.FieldKeyMsg:   "@message",
		},
	})

	return l
}

func readJunitFile(filename string) (Testsuite, error) {
	var testsuite Testsuite
	f, err := os.Open(filename)
	if err != nil {
		return testsuite, fmt.Errorf("Failed to open junit file: %s", err)
	}
	defer f.Close()

	byteValue, err := ioutil.ReadAll(f)
	if err != nil {
		return testsuite, fmt.Errorf("Failed to read junit file: %s", err)
	}

	s := string(byteValue)
	s = strings.ReplaceAll(s, "", "    ")

	if err := xml.Unmarshal([]byte(s), &testsuite); err != nil {
		return testsuite, fmt.Errorf("Failed to marshall junit file: %s", err)
	}

	return testsuite, nil
}

func processJunitFile(testDirectory string, file os.FileInfo, logger *logrus.Entry) (string, []string, error) {
	testfile := ""
	testcases := []string{}

	logger.Info("Processing")
	testsuite, err := readJunitFile(path.Join(testDirectory, file.Name()))
	if err != nil {
		logger.Warn("Error reading file")
		return testfile, testcases, fmt.Errorf("Error reading file: %s", err.Error())
	}

	for _, property := range testsuite.Properties.Property {
		if property.Name == "BATS_CWD" {
			testfile = path.Join(property.Value, testsuite.Name)
		}
	}

	if testfile == "" {
		logger.Warn("Unable to generate testfile path")
		return testfile, testcases, errors.New("Unable to generate testfile path")
	}

	for _, testcase := range testsuite.Testcase {
		l := logger.WithField("testcase", testcase.Name)
		if testcase.Skipped != "" {
			l.WithField("status", "skipped").Info("Adding skipped testcase")
			testcases = append(testcases, testcase.Name)
			continue
		}

		if testcase.Failure.Text != "" {
			l.WithField("status", "failed").Info("Adding failed testcase")
			testcases = append(testcases, testcase.Name)
			continue
		}
	}

	return testfile, testcases, nil
}

func writeSliceToFile(filename string, lines []string) error {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	if err = w.Flush(); err != nil {
		return err
	}

	file.Chmod(0700)

	return nil
}

func executeBatsCommands(commandMap map[string][][]string, testDirectory string, logger *logrus.Entry) error {
	var result error
	for testfile, commands := range commandMap {
		l := logger.WithField("testfile", testfile)
		for _, command := range commands {
			testcase := command[0]

			args := make([]interface{}, 0)
			args = append(args, "--filter", escapeTestcase(testcase), command[1])

			lc := l.WithField("testcase", testcase)
			lc.WithField("bats", args).Info("Executing bats command")
			startTime := time.Now()
			if err := sh.Command("bats", args...).Run(); err != nil {
				result = multierror.Append(result, err)
				continue
			}
			endTime := time.Now()
			runTime := endTime.Sub(startTime)

			if err := updateTestFile(testfile, testDirectory, testcase, runTime, lc); err != nil {
				result = multierror.Append(result, err)
			}
		}
	}

	return result
}

func updateTestFile(testfile string, testDirectory string, testcase string, runTime time.Duration, logger *logrus.Entry) error {
	logger.Info("Updating testfile for testcase")

	filename := path.Join(testDirectory, testfile)
	testsuite, err := readJunitFile(filename)
	if err != nil {
		return err
	}

	for i, t := range testsuite.Testcase {
		if t.Name != testcase {
			continue
		}

		l := logger.WithField("testcase", t.Name)
		l.Info("Updating testcase")
		testsuite.Testcase[i].Time = fmt.Sprintf("%v", math.Round(runTime.Seconds()))
		testsuite.Testcase[i].Skipped = ""
		testsuite.Testcase[i].Failure.Text = ""
		testsuite.Testcase[i].Failure.Type = ""
	}

	b, err := xml.MarshalIndent(testsuite, "", "   ")
	if err != nil {
		return fmt.Errorf("Failed to marshal testsuite to string: %s", err.Error())
	}

	s := strings.ReplaceAll(string(b), "&#xA;", "")
	s = strings.ReplaceAll(s, "<failure type=\"\"></failure>", "")
	s = strings.ReplaceAll(s, "<skipped></skipped>", "")

	output := []string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " ")
		if line == "" {
			continue
		}
		output = append(output, line)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("Failed to open junit file for writing: %s", err.Error())
	}
	defer f.Close()

	if _, err := f.Write([]byte(strings.Join(output, "\n"))); err != nil {
		return fmt.Errorf("Failed to write junit file: %s", err.Error())
	}

	return nil
}

func escapeTestcase(testcase string) string {
	escapedTestName := strings.ReplaceAll(testcase, "(", "\\(")
	return strings.ReplaceAll(escapedTestName, ")", "\\)")

}

func main() {
	args := flag.NewFlagSet("bats-retry", flag.ExitOnError)
	var execute *bool = args.Bool("execute", false, "whether to execute bats commands directly")

	args.Parse(os.Args[1:])
	testDirectory := args.Arg(0)
	testScript := args.Arg(1)

	if testDirectory == "" {
		logger.Error("No test directory specified")
		os.Exit(1)
	}

	if testScript == "" && !*execute {
		logger.Error("No test script location specified")
		os.Exit(1)
	}

	l := logger.WithField("test-directory", testDirectory)
	files, err := ioutil.ReadDir(testDirectory)
	if err != nil {
		l.WithField("error", err.Error()).Error("Error reading test directory")
		os.Exit(1)
	}

	validFiles := []os.FileInfo{}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".xml") {
			continue
		}

		validFiles = append(validFiles, file)
	}

	if len(validFiles) == 0 {
		l.Info("No testsuites found")
		os.Exit(0)
	}

	lines := []string{"#!/usr/bin/env bash", "set -eo pipefail", ""}
	batsCommands := map[string][][]string{}
	for _, file := range validFiles {
		lf := l.WithField("file", file.Name())
		testfile, newTests, err := processJunitFile(testDirectory, file, lf)
		if err != nil {
			lf.WithField("error", err.Error()).Error("Error processing file")
			os.Exit(1)
		}

		batsCommands[file.Name()] = [][]string{}
		for _, test := range newTests {
			lines = append(lines, fmt.Sprintf("bats --filter '%s' %s", escapeTestcase(test), testfile))
			batsCommands[file.Name()] = append(batsCommands[file.Name()], []string{test, testfile})
		}
	}

	if *execute {
		if err := executeBatsCommands(batsCommands, testDirectory, l); err != nil {
			l.WithField("error", err.Error()).Error("Error executing bats commands")
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := writeSliceToFile(testScript, lines); err != nil {
		l.WithField("error", err.Error()).Error("Error writing file")
		os.Exit(1)
	}
}
