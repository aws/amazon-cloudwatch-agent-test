package local_workflow

import (
	"encoding/json"
	"log"
	"os"
)

const ConfigTfvarsFilename = "config_ignore.tfvars"

type Config struct {
	TerraformPath        string `json:"terraformPath"`
	S3Bucket             string `json:"s3Bucket"`
	CwaGithubSha         string `json:"cwaGithubSha"`
	GithubTestRepo       string `json:"githubTestRepo"`
	GithubTestRepoBranch string `json:"githubTestRepoBranch"`
	PluginTests          string `json:"pluginTests"`
	TestDir              string `json:"testDir"`
	Os                   string `json:"os"`
	Family               string `json:"family"`
	TestType             string `json:"testType"`
	Arc                  string `json:"arc"`
	InstanceType         string `json:"instanceType"`
	Ami                  string `json:"ami"`
	BinaryName           string `json:"binaryName"`
	User                 string `json:"user"`
	InstallAgent         string `json:"installAgent"`
	CaCertPath           string `json:"caCertPath"`
	ValuesPerMinute      int    `json:"valuesPerMinute"`
}

func FetchConfig() Config {
	const configPath = "config.json"
	raw, err := os.ReadFile(configPath)
	PanicIfError(err)
	var config Config
	err = json.Unmarshal(raw, &config)
	if err != nil {
		log.Fatal("Error during json.Unmarshall() in fetchConfig(): ", err)
	}
	return config
}
