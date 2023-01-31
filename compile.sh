test() {
  local root=$1
  testName=${root##*/}
  echo "compiling $testName"
  go test -c "$root" -o ./test_binaries/"$testName".test
}

dfs() {
  local root=$1

  if [ ! -d "$root" ]; then
    return
  fi

  local hasTest=false
  for p in "$root"/* ; do
    local path=$p
    if [[ $path == *_test.go ]]; then
#      echo "$root"
      hasTest=true
    fi
    dfs "$path"
  done

  if $hasTest; then
      test $root
  fi
}

dfs "./test"

