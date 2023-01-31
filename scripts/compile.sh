# compile.sh looks through a directory and compiles all of the tests

mkdir -p ./test_binaries

# compiles a test folder
# $1=filepath to a directory containing a go test
compile() {
  local testDir=$1
  testName=${testDir##*/}
  echo "compiling $testName"
  # compiles a test without running it
  # https://pkg.go.dev/cmd/go#hdr-Test_packages
  go test -c "$testDir" -o "./test_binaries/$testName.test"
}

# navigates the filesystem to identify folders with tests
# $1=filepath of the current directory we searching through
dfs() {
  local dir=$1
  local hasTest=false

  # base case: ensure that dir is a directory
  if [ ! -d "$dir" ]; then
    return
  fi

  # look for tests
  for path in "$dir"/* ; do
    if [[ $path == *_test.go ]]; then
      hasTest=true
    fi
    # check every path
    dfs "$path"
  done

  if $hasTest; then
    compile "$dir"
  fi
}

# entry point
dfs "./test"
