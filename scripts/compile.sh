mkdir -p ./test_binaries

TEST_BINARIES_DIR="./test_binaries"

# compiles a test folder
# $1=filepath to a directory containing a go test
compile() {
  local root=$1
  testName=${root##*/}
  echo "compiling $testName"
  # compiles a test without running it
  # https://pkg.go.dev/cmd/go#hdr-Test_packages
  go test -c "$root" -o "$TEST_BINARIES_DIR/$testName.test"
}

# navigates the filesystem to identify folders with tests
# $1=filepath of the current directory we searching through
dfs() {
  local root=$1
  local hasTest=false

  # base case: ensure that root is a directory
  if [ ! -d "$root" ]; then
    return
  fi

  # look for tests
  for path in "$root"/* ; do
    if [[ $path == *_test.go ]]; then
      hasTest=true
    fi
    # check every path
    dfs "$path"
  done

  if $hasTest; then
    compile "$root"
  fi
}

# entry point
dfs "./test"
