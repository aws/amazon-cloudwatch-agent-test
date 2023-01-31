TEST_BINARIES_DIR="./test_binaries"

# compiles
# $1=filepath to a directory containing a go test
compile() {
  local root=$1
  testName=${root##*/}
  echo "compiling $testName"
  go test -c "$root" -o "$TEST_BINARIES_DIR/$testName.test"
}

# navigates the fs to identify folders with tests
# $1=filepath of the current directory we searching through
dfs() {
  local root=$1
  local hasTest=false

  # checks that root is a directory
  if [ ! -d "$root" ]; then
    return
  fi

  for path in "$root"/* ; do
    if [[ $path == *_test.go ]]; then
      hasTest=true
    fi
    dfs "$path"
  done

  if $hasTest; then
    compile "$root"
  fi
}

dfs "./test"
