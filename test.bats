#!/usr/bin/env bats

export SYSTEM_NAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
export BATS_RETRY_BIN="build/$SYSTEM_NAME/bats-retry"

setup_file() {
  mkdir -p test-output
  make prebuild $BATS_RETRY_BIN
}

teardown_file() {
  rm -r test-output
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

@test "no tests" {
  run /bin/bash -c "$BATS_RETRY_BIN fixtures/empty test-output/script"
  assert_success
  assert_output_contains "No testsuites found"
}

@test "args" {
  run /bin/bash -c "$BATS_RETRY_BIN"
  assert_failure
  assert_output_contains "No test directory specified"

  run /bin/bash -c "$BATS_RETRY_BIN fixtures/empty"
  assert_failure
  assert_output_contains "No test script location specified"
}
