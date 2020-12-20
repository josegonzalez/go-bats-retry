#!/usr/bin/env bats

export SYSTEM_NAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
export BATS_RETRY_BIN="build/$SYSTEM_NAME/bats-retry"

setup_file() {
  make prebuild $BATS_RETRY_BIN
}

teardown_file() {
  make clean
}

# test functions
flunk() {
  {
    if [[ "$#" -eq 0 ]]; then
      cat -
    else
      echo "$*"
    fi
  }
  return 1
}

assert_equal() {
  if [[ "$1" != "$2" ]]; then
    {
      echo "expected: $1"
      echo "actual:   $2"
    } | flunk
  fi
}

# ShellCheck doesn't know about $output from Bats
# shellcheck disable=SC2154
assert_output() {
  local expected
  if [[ $# -eq 0 ]]; then
    expected="$(cat -)"
  else
    expected="$1"
  fi
  assert_equal "$expected" "$output"
}

# ShellCheck doesn't know about $output from Bats
# shellcheck disable=SC2154
assert_output_contains() {
  local input="$output"
  local expected="$1"
  local count="${2:-1}"
  local found=0
  until [ "${input/$expected/}" = "$input" ]; do
    input="${input/$expected/}"
    let found+=1
  done
  assert_equal "$count" "$found"
}

# ShellCheck doesn't know about $status from Bats
# shellcheck disable=SC2154
# shellcheck disable=SC2120
assert_success() {
  if [[ "$status" -ne 0 ]]; then
    flunk "command failed with exit status $status"
  elif [[ "$#" -gt 0 ]]; then
    assert_output "$1"
  fi
}

# ShellCheck doesn't know about $status from Bats
# shellcheck disable=SC2154
# shellcheck disable=SC2120
assert_failure() {
  if [[ "$status" -eq 0 ]]; then
    flunk "expected failed exit status"
  elif [[ "$#" -gt 0 ]]; then
    assert_output "$1"
  fi
}

@test "args" {
  run /bin/bash -c "$BATS_RETRY_BIN"
  echo "output: $output"
  echo "status: $status"
  assert_failure
  assert_output_contains "No test directory specified"

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_failure

  run /bin/bash -c "$BATS_RETRY_BIN fixtures/empty"
  echo "output: $output"
  echo "status: $status"
  assert_failure
  assert_output_contains "No test script location specified"

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_failure
}

@test "[invalid] directory" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/missing validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_failure
  assert_output_contains "no such file or directory"

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_failure
}

@test "[invalid] file" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/invalid-xml validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_failure
  assert_output_contains "Error processing file"

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_failure
}

@test "[invalid] no test suites" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/invalid-no-tests validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success
  assert_output_contains "No testsuites found"

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_failure
}

@test "[no failures] single file" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/successful validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_success
}

@test "[no failures] multiple files" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/successful-multiple validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_success
}

@test "[failures] failed" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/failure validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_success

  run /bin/bash -c "cat validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success
  assert_output_contains "nginx:set proxy-busy-buffers-size"

  run /bin/bash -c "cat validation/script | wc -l"
  echo "output: $output"
  echo "status: $status"
  assert_equal "$output" 4
}

@test "[failures] skipped" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/failure-skipped validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_success

  run /bin/bash -c "cat validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success
  assert_output_contains "nginx:set proxy-busy-buffers-size"
  assert_output_contains "with SSL and unrelated domain"
  assert_output_contains "wildcard SSL"

  run /bin/bash -c "cat validation/script | wc -l"
  echo "output: $output"
  echo "status: $status"
  assert_equal "$output" 6
}

@test "[failures] combined" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/failure-combined validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success

  run test -x validation/script
  echo "output: $output"
  echo "status: $status"
  assert_success

  run /bin/bash -c "cat validation/script"
  echo "output: $output"
  echo "status: $status"
  assert_success
  assert_output_contains "nginx:set whatever"
  assert_output_contains "nginx:set proxy-busy-buffers-size"
  assert_output_contains "with SSL and unrelated domain"
  assert_output_contains "wildcard SSL"

  run /bin/bash -c "cat validation/script | wc -l"
  echo "output: $output"
  echo "status: $status"
  assert_equal "$output" 7
}
