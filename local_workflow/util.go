package local_workflow

import (
	"encoding/json"
	"fmt"
	"log"
)

func PrettyPrint(data interface{}) {
	pretty, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		log.Fatal("prettyPrint() failed: ", err)
	}
	fmt.Printf("%v\n", string(pretty))
}

func PanicIfError(err error) {
	if err != nil {
		panic(err)
	}
}

func DoNothing(data any) {}
