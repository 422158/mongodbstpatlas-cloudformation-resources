// Code generated by 'cfn generate', changes will be undone by the next invocation. DO NOT EDIT.
// Updates to this type are made my editing the schema file and executing the 'generate' command.
package resource

// Model is autogenerated from the json schema
type Model struct {
	CfnPrimaryIdentifier *string           `json:",omitempty"`
	AwsKms               *AwsKms           `json:",omitempty"`
	ApiKeys              *ApiKeyDefinition `json:",omitempty"`
	ProjectId            *string           `json:",omitempty"`
}

// AwsKms is autogenerated from the json schema
type AwsKms struct {
	AccessKeyID         *string `json:",omitempty"`
	CustomerMasterKeyID *string `json:",omitempty"`
	Enabled             *bool   `json:",omitempty"`
	Region              *string `json:",omitempty"`
	SecretAccessKey     *string `json:",omitempty"`
}

// ApiKeyDefinition is autogenerated from the json schema
type ApiKeyDefinition struct {
	PublicKey  *string `json:",omitempty"`
	PrivateKey *string `json:",omitempty"`
}
