package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/database-user/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/davecgh/go-spew/spew"
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

	cfnid := buildUserCfnIdentifier(currentModel.ProjectId, currentModel.Username)
	currentModel.UserCfnIdentifier = &cfnid

	_, _, err = client.DatabaseUsers.Create(context.Background(), groupID, &user)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error creating database user: %s", err)
	}

	// putting api keys and project name into parameter store (needed for read operation)
	_, err = putParameterIntoParameterStore(currentModel.UserCfnIdentifier, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId, Username: currentModel.Username, DatabaseName: currentModel.DatabaseName}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Create Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Read handles the Read event from the Cloudformation service.
func Read(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	params, err := getParameterFromParameterStore(currentModel.UserCfnIdentifier, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	client, err := util.CreateMongoDBClient(*params.ApiKeys.PublicKey, *params.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	databaseUser, _, err := client.DatabaseUsers.Get(context.Background(), *params.DatabaseName, *params.ProjectId, *params.Username)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error fetching database user (%s): %s", *params.Username, err)
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
	currentModel.UserCfnIdentifier = &cfnid

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

	currentModel.UserCfnIdentifier = prevModel.UserCfnIdentifier

	// putting api keys and project name into parameter store (needed for read operation)
	_, err = putParameterIntoParameterStore(currentModel.UserCfnIdentifier, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId, Username: currentModel.Username, DatabaseName: currentModel.DatabaseName}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
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
	username := url.QueryEscape(*currentModel.Username)
	dbName := *currentModel.DatabaseName

	resp, err := client.DatabaseUsers.Delete(context.Background(), dbName, groupID, username)
	userDeletedSuccess := true
	if err != nil || resp.StatusCode != 204 {
		userDeletedSuccess = false
		// if resp.StatusCode == 404 {
		// 	userDeletedSuccess = true
		// }
	}

	_, respErr := deleteParameterFromParameterStore(currentModel.UserCfnIdentifier, req.Session)
	parameterDeletedSuccess := true
	if respErr != nil {
		parameterDeletedSuccess = false
	}

	if !userDeletedSuccess && !parameterDeletedSuccess {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete both user and parameter from store for user %s\n%s\n%s", *currentModel.UserCfnIdentifier, err, respErr)
	}
	if !userDeletedSuccess {
		// bytes, err := httputil.DumpResponse(resp.Response, true)
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete user with id %s\n%s\nResponse:%s", *currentModel.UserCfnIdentifier, err, spew.Sprint(resp))
	}
	if !parameterDeletedSuccess {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete parameter for user with id %s\n%s", *currentModel.UserCfnIdentifier, respErr)
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

func buildUserCfnIdentifier(projectId *string, userName *string) string {
	cfnid := fmt.Sprintf("%s-%s-%s", "user", strings.ToLower(strings.Replace(strings.Replace(*userName, ":", "", -1), "/", "_", -1)), *projectId)
	return cfnid
}

type ParameterToBePersistedSpec struct {
	ApiKeys      *ApiKeyDefinition
	ProjectId    *string
	Username     *string
	DatabaseName *string
}

func putParameterIntoParameterStore(resourcePrimaryIdentifier *string, params *ParameterToBePersistedSpec, session *session.Session) (*ssm.PutParameterOutput, error) {
	ssmClient, err := util.CreateSSMClient(session)
	if err != nil {
		return nil, err
	}
	// transform api keys to json string
	parameterName := buildApiKeyParameterName(*resourcePrimaryIdentifier)
	byteParams, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	stringifiedParams := string(byteParams)
	parameterType := "SecureString"
	overwrite := true
	putParamOutput, err := ssmClient.PutParameter(&ssm.PutParameterInput{Name: &parameterName, Value: &stringifiedParams, Type: &parameterType, Overwrite: &overwrite})
	if err != nil {
		return nil, fmt.Errorf("Unable to put parameter %s: %s", parameterName, err)
	}

	return putParamOutput, nil
}

func deleteParameterFromParameterStore(resourcePrimaryIdentifier *string, session *session.Session) (*ssm.DeleteParameterOutput, error) {
	ssmClient, err := util.CreateSSMClient(session)
	if err != nil {
		return nil, err
	}
	parameterName := buildApiKeyParameterName(*resourcePrimaryIdentifier)

	deleteParamOutput, err := ssmClient.DeleteParameter(&ssm.DeleteParameterInput{Name: &parameterName})
	if err != nil {
		return nil, err
	}

	return deleteParamOutput, nil
}

func getParameterFromParameterStore(resourcePrimaryIdentifier *string, session *session.Session) (*ParameterToBePersistedSpec, error) {
	ssmClient, err := util.CreateSSMClient(session)
	if err != nil {
		return nil, err
	}
	parameterName := buildApiKeyParameterName(*resourcePrimaryIdentifier)
	decrypt := true
	getParamOutput, err := ssmClient.GetParameter(&ssm.GetParameterInput{Name: &parameterName, WithDecryption: &decrypt})
	if err != nil {
		return nil, err
	}

	var params ParameterToBePersistedSpec
	err = json.Unmarshal([]byte(*getParamOutput.Parameter.Value), &params)
	if err != nil {
		return nil, err
	}
	return &params, nil
}

func buildApiKeyParameterName(resourcePrimaryIdentifier string) string {
	// this is strictly coupled with permissions for handlers, changing this means changing permissions in handler
	// moreover changing this might cause polution in parameter store -  be sure you know what you are doing
	parameterStorePrefix := "mongodbstpatlasv1databaseuser"
	return fmt.Sprintf("%s-%s", parameterStorePrefix, resourcePrimaryIdentifier)
}
