// Code generated by 'cfn generate', changes will be undone by the next invocation. DO NOT EDIT.
// Updates to this type are made my editing the schema file and executing the 'generate' command.
package resource

import "github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"

// TypeConfiguration is autogenerated from the json schema
type TypeConfiguration struct {
}

// Configuration returns a resource's configuration.
func Configuration(req handler.Request) (*TypeConfiguration, error) {
	// Populate the type configuration
	typeConfig := &TypeConfiguration{}
	if err := req.UnmarshalTypeConfig(typeConfig); err != nil {
		return typeConfig, err
	}
	return typeConfig, nil
}
