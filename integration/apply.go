package integration

import (
	"fmt"
	"path"
)

func BuildTerraformCommand(rootDir, terraformRelativePath, varsAbsolutePath string) []string {
	terraformAbsolutePath := path.Join(rootDir, terraformRelativePath)
	commands := []string{
		fmt.Sprintf("cd %v", terraformAbsolutePath),
		"terraform init",
		fmt.Sprintf(`terraform apply --auto-approve -var-file="%v"`, varsAbsolutePath),
		"terraform destroy --auto-approve",
	}
	return commands
}
