package integration

import (
	"encoding/json"
	"fmt"
	"log"
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

func DoNothing(data any) {}
