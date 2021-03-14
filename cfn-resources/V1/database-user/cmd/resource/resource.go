package resource

import (
	"context"
	"fmt"
	"log"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/database-user/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"go.mongodb.org/atlas/mongodbatlas"
)

// Create handles the Create event from the Cloudformation service.
func Create(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	// mongo roles for the user
	var roles []mongodbatlas.Role
	for _, r := range currentModel.Roles {

		role := mongodbatlas.Role{RoleName: *r.RoleName}

		if r.CollectionName != nil {
			role.CollectionName = *r.CollectionName
		}
		if r.DatabaseName != nil {
			role.DatabaseName = *r.DatabaseName
		}

		roles = append(roles, role)
	}
	// mongo scopes for the user
	var scopes []mongodbatlas.Scope
	for _, s := range currentModel.Scopes {

		scope := mongodbatlas.Scope{
			Type: *s.Type,
			Name: *s.Name,
		}

		scopes = append(scopes, scope)
	}
	// mongo labels for user
	var labels []mongodbatlas.Label
	for _, l := range currentModel.Labels {

		label := mongodbatlas.Label{
			Key:   *l.Key,
			Value: *l.Value,
		}
		labels = append(labels, label)
	}

	groupID := *currentModel.ProjectId

	// basic user object
	user := mongodbatlas.DatabaseUser{
		Roles:        roles,
		Scopes:       scopes,
		GroupID:      groupID,
		Username:     *currentModel.Username,
		DatabaseName: *currentModel.DatabaseName,
		Labels:       labels,
	}

	if currentModel.Password != nil {
		user.Password = *currentModel.Password
	}

	if currentModel.AwsIAMType != nil {
		user.AWSIAMType = *currentModel.AwsIAMType
	}

	if currentModel.LdapAuthType != nil {
		user.LDAPAuthType = *currentModel.LdapAuthType
	}

	log.Printf("Arguments: Project ID: %s, Request %#+v", groupID, &user)
	cfnid := fmt.Sprintf("%s-%s", user.GroupID, user.Username)
	currentModel.UserCNFIdentifier = &cfnid
	log.Printf("UserCFNIdentifier: %s", cfnid)

	_, _, err = client.DatabaseUsers.Create(context.Background(), groupID, &user)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error creating database user: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Create Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Read handles the Read event from the Cloudformation service.
func Read(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	groupID := *currentModel.ProjectId
	username := *currentModel.Username
	dbName := *currentModel.DatabaseName
	databaseUser, _, err := client.DatabaseUsers.Get(context.Background(), dbName, groupID, username)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error fetching database user (%s): %s", username, err)
	}

	currentModel.LdapAuthType = &databaseUser.LDAPAuthType
	currentModel.AwsIAMType = &databaseUser.AWSIAMType

	// reading roles from remote
	var roles []RoleDefinition
	for _, r := range databaseUser.Roles {
		role := RoleDefinition{
			CollectionName: &r.CollectionName,
			DatabaseName:   &r.DatabaseName,
			RoleName:       &r.RoleName,
		}

		roles = append(roles, role)
	}
	currentModel.Roles = roles

	// reading scopes from remote
	var scopes []ScopeDefinition
	for _, s := range databaseUser.Scopes {
		scope := ScopeDefinition{
			Type: &s.Type,
			Name: &s.Name,
		}

		scopes = append(scopes, scope)
	}
	currentModel.Scopes = scopes

	// reading labels from remote
	var labels []LabelDefinition
	for _, l := range databaseUser.Labels {
		label := LabelDefinition{
			Key:   &l.Key,
			Value: &l.Value,
		}

		labels = append(labels, label)
	}
	currentModel.Labels = labels

	cfnid := fmt.Sprintf("%s-%s", *currentModel.ProjectId, *currentModel.Username)
	currentModel.UserCNFIdentifier = &cfnid

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Read Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Update handles the Update event from the Cloudformation service.
func Update(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	var roles []mongodbatlas.Role
	for _, r := range currentModel.Roles {
		role := mongodbatlas.Role{RoleName: *r.RoleName}

		if r.CollectionName != nil {
			role.CollectionName = *r.CollectionName
		}
		if r.DatabaseName != nil {
			role.DatabaseName = *r.DatabaseName
		}
		roles = append(roles, role)
	}

	var scopes []mongodbatlas.Scope
	for _, s := range currentModel.Scopes {

		scope := mongodbatlas.Scope{
			Type: *s.Type,
			Name: *s.Name,
		}

		scopes = append(scopes, scope)
	}

	var labels []mongodbatlas.Label
	for _, l := range currentModel.Labels {

		label := mongodbatlas.Label{
			Key:   *l.Key,
			Value: *l.Value,
		}
		labels = append(labels, label)
	}

	groupID := *currentModel.ProjectId
	username := *currentModel.Username

	user := mongodbatlas.DatabaseUser{
		Roles:        roles,
		Scopes:       scopes,
		GroupID:      groupID,
		Username:     username,
		DatabaseName: *currentModel.DatabaseName,
		Labels:       labels,
	}

	if currentModel.Password != nil {
		user.Password = *currentModel.Password
	}

	if currentModel.AwsIAMType != nil {
		user.AWSIAMType = *currentModel.AwsIAMType
	}

	if currentModel.LdapAuthType != nil {
		user.LDAPAuthType = *currentModel.LdapAuthType
	}

	_, _, err = client.DatabaseUsers.Update(context.Background(), groupID, username,
		&user)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error updating database user (%s): %s", username, err)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Update Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Delete handles the Delete event from the Cloudformation service.
func Delete(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	groupID := *currentModel.ProjectId
	username := *currentModel.Username
	dbName := *currentModel.DatabaseName

	_, err = client.DatabaseUsers.Delete(context.Background(), dbName, groupID, username)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error deleting database user (%s): %s", username, err)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Delete Complete",
		ResourceModel:   currentModel,
	}, nil
}

// List NOOP
func List(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "List Complete",
		ResourceModel:   currentModel,
	}, nil
}
