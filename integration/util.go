package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"unicode"
)

func PrettyPrint(data interface{}) {
	pretty, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		log.Fatal("prettyPrint() failed: ", err)
	}
	fmt.Printf("%v\n", string(pretty))
}

func LogFatalIfError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// ConvertCamelToSnakeCase converts a string from camel to snake case
// e.g. helloThere => hello_there
func ConvertCamelToSnakeCase(camel string) string {
	var words []string
	lo := 0
	for hi, char := range camel {
		if unicode.IsUpper(char) {
			word := strings.ToLower(camel[lo:hi])
			if len(word) > 0 {
				words = append(words, word)
			}
			lo = hi
		}
	}
	// append last word
	words = append(words, strings.ToLower(camel[lo:]))
	return strings.Join(words, "_")
}

func GetRootDir() string {
	wd, _ := os.Getwd()
	rootDir := path.Join(wd, "../")
	return rootDir
}

func WriteToPath(path, payload string) {
	raw := []byte(payload)
	err := os.WriteFile(path, raw, 0644)
	if err != nil {
		panic(err)
	}
}

func ExecuteScript(filepath string) {
	err := exec.Command("chmod", "+x", filepath).Run()
	LogFatalIfError(err)
	cmd := exec.Command("/bin/sh", filepath)

	stderr, err := cmd.StderrPipe()
	cmd.Start()
	LogFatalIfError(err)

	scanner := bufio.NewScanner(stderr)
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		m := scanner.Text()
		fmt.Println(m)
	}
	cmd.Wait()
}

func DoNothing(data any) {}
