package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

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

func processFile(testDirectory string, file os.FileInfo, logger *logrus.Entry) ([]string, error) {
	testcases := []string{}
	logger.Info("Processing")
	xmlFile, err := os.Open(path.Join(testDirectory, file.Name()))
	if err != nil {
		return testcases, err
	}
	defer xmlFile.Close()

	byteValue, err := ioutil.ReadAll(xmlFile)
	if err != nil {
		return testcases, err
	}

	s := string(byteValue)
	s = strings.ReplaceAll(s, "", "    ")

	var testsuite Testsuite
	if err := xml.Unmarshal([]byte(s), &testsuite); err != nil {
		return testcases, err
	}

	for _, testcase := range testsuite.Testcase {
		l := logger.WithField("testcase", testcase.Name)
		if testcase.Skipped != "" {
			l.WithField("status", "skipped").Info("skipped")
			testcases = append(testcases, testcase.Name)
			continue
		}

		if testcase.Failure.Text != "" {
			l.WithField("status", "failed").Info("failed")
			testcases = append(testcases, testcase.Name)
			continue
		}
	}

	return testcases, nil
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

func main() {
	args := flag.NewFlagSet("bats-retry", flag.ExitOnError)
	args.Parse(os.Args[1:])
	testDirectory := args.Arg(0)
	testScript := args.Arg(1)

	if testDirectory == "" {
		logger.Error("No test directory specified")
		os.Exit(1)
	}

	if testScript == "" {
		logger.Error("No test script location specified")
		os.Exit(1)
	}

	l := logger.WithField("test-directory", testDirectory)
	files, err := ioutil.ReadDir(testDirectory)
	if err != nil {
		l.Error(err)
		os.Exit(1)
	}

	if len(files) == 0 {
		logger.Info("No testsuites found")
		os.Exit(0)
	}

	lines := []string{"#!/usr/bin/env bash", "set -eo pipefail", ""}
	for _, file := range files {
		lf := l.WithField("file", file.Name())
		newTests, err := processFile(testDirectory, file, lf)
		if err != nil {
			logger.Error(err)
			os.Exit(1)
		}

		for _, test := range newTests {
			test = strings.ReplaceAll(test, "(", "\\(")
			test = strings.ReplaceAll(test, ")", "\\)")
			lines = append(lines, fmt.Sprintf("bats --filter '%s' %s", test, path.Join(testDirectory, file.Name())))
		}
	}

	if err := writeSliceToFile(testScript, lines); err != nil {
		l.Error(err)
		os.Exit(1)
	}
}
