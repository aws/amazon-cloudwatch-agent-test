package local_workflow

import (
	"fmt"
	"os"
	"path"
	"reflect"
)

const varsFilename = "config_ignore.tfvars"

func WriteVarsFile(config Config) string {
	configVars := fetchVars(config)
	file, err := os.Create(varsFilename)
	LogFatalIfError(err)
	defer file.Close()
	for _, configVar := range configVars {
		_, err := file.WriteString(configVar + "\n")
		LogFatalIfError(err)
	}

	wd, err := os.Getwd()
	LogFatalIfError(err)
	return path.Join(wd, varsFilename)
}

func fetchVars(config Config) []string {
	var vars []string
	values := reflect.ValueOf(config)
	types := values.Type()
	for i := 0; i < values.NumField(); i++ {
		key := types.Field(i).Name
		key = ConvertCamelToSnakeCase(key)
		val := values.Field(i).Interface()
		configVar := buildTfvar(key, val)
		vars = append(vars, configVar)
	}
	return vars
}

func buildTfvar(key string, val any) string {
	const (
		stringFmt  = `%v="%v"`
		defaultFmt = "%v=%v"
	)
	if _, ok := val.(string); ok {
		return fmt.Sprintf(stringFmt, key, val)
	} else {
		return fmt.Sprintf(defaultFmt, key, val)
	}
}
