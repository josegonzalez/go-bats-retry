# bats-retry

Retry skipped and failing bats tests.

## Usage

By default, `bats-retry` can be used to create a test script:

```shell
# create a test script
bats-retry path/to/junit/output path/to/test/script

# run the script
path/to/test/script
```

The alternative is to execute the commands directly. This method will also update the junit xml file to remove now passing tests.

```shell
# create a test script
bats-retry --execute path/to/junit/output
```

