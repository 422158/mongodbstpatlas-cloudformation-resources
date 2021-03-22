package resource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/network-peering/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"

	// "github.com/davecgh/go-spew/spew"

	matlasClient "go.mongodb.org/atlas/mongodbatlas"
)

// Create handles the Create event from the Cloudformation service.
func Create(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	if _, ok := req.CallbackContext["stateName"]; ok {
		return validateProgress(client, currentModel, []string{"PENDING_ACCEPTANCE", "FINALIZING", "AVAILABLE"})
	}

	defaultProviderName := "AWS"
	projectID := *currentModel.ProjectId
	peerRequest := matlasClient.Peer{
		ContainerID: *currentModel.ContainerId,
	}

	region := currentModel.AccepterRegionName
	if region == nil || *region == "" {
		return handler.ProgressEvent{}, fmt.Errorf("error creating network peering: `accepter_region_name` must be set")
	}
	awsAccountId := currentModel.AwsAccountId
	if awsAccountId == nil || *awsAccountId == "" {
		return handler.ProgressEvent{}, fmt.Errorf("error creating network peering: `aws_account_id` must be set")
	}
	rtCIDR := currentModel.RouteTableCidrBlock
	if rtCIDR == nil || *rtCIDR == "" {
		return handler.ProgressEvent{}, fmt.Errorf("error creating network peering: `route_table_cidr_block` must be set")
	}
	vpcID := currentModel.VpcId
	if vpcID == nil || *vpcID == "" {
		return handler.ProgressEvent{}, fmt.Errorf("error creating network peering: `vpc_id` must be set")
	}
	providerName := currentModel.ProviderName
	if providerName == nil || *providerName == "" {
		providerName = &defaultProviderName
	}
	peerRequest.AccepterRegionName = *region
	peerRequest.AWSAccountID = *awsAccountId
	peerRequest.RouteTableCIDRBlock = *rtCIDR
	peerRequest.VpcID = *vpcID
	peerRequest.ProviderName = *providerName

	peerResponse, _, err := client.Peers.Create(context.Background(), projectID, &peerRequest)

	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error creating network peering: %s", err)
	}

	currentModel.Id = &peerResponse.ID

	// putting api keys and project name into parameter store (needed for read operation)
	_, err = putParameterIntoParameterStore(currentModel.Id, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus:      handler.InProgress,
		Message:              "Create complete",
		ResourceModel:        currentModel,
		CallbackDelaySeconds: 10,
		CallbackContext: map[string]interface{}{
			"stateName": peerResponse.StatusName,
		},
	}, nil
}

// Read handles the Read event from the Cloudformation service.
func Read(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {

	params, err := getParameterFromParameterStore(currentModel.Id, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	client, err := util.CreateMongoDBClient(*params.ApiKeys.PublicKey, *params.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	peerID := *currentModel.Id

	peerResponse, _, err := client.Peers.Get(context.Background(), *params.ProjectId, peerID)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error reading peer with id(project: %s, peer: %s): %s", *params.ProjectId, peerID, err)
	}

	currentModel.AccepterRegionName = &peerResponse.AccepterRegionName
	currentModel.AwsAccountId = &peerResponse.AWSAccountID
	currentModel.RouteTableCidrBlock = &peerResponse.RouteTableCIDRBlock
	currentModel.VpcId = &peerResponse.VpcID
	currentModel.ConnectionId = &peerResponse.ConnectionID
	currentModel.ErrorStateName = &peerResponse.ErrorStateName
	currentModel.StatusName = &peerResponse.StatusName
	currentModel.ProviderName = &peerResponse.ProviderName

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

	if _, ok := req.CallbackContext["stateName"]; ok {
		return validateProgress(client, currentModel, []string{"PENDING_ACCEPTANCE", "FINALIZING", "AVAILABLE"})
	}

	projectID := *currentModel.ProjectId
	peerID := *currentModel.Id
	peerRequest := matlasClient.Peer{}

	region := currentModel.AccepterRegionName
	if region != nil {
		peerRequest.AccepterRegionName = *region
	}
	accountID := currentModel.AwsAccountId
	if accountID != nil {
		peerRequest.AWSAccountID = *accountID
	}
	peerRequest.ProviderName = "AWS"
	rtTableBlock := currentModel.RouteTableCidrBlock
	if rtTableBlock != nil {
		peerRequest.RouteTableCIDRBlock = *rtTableBlock
	}
	vpcId := currentModel.VpcId
	if vpcId != nil {
		peerRequest.VpcID = *vpcId
	}
	peerResponse, _, err := client.Peers.Update(context.Background(), projectID, peerID, &peerRequest)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error updating peer with id(project: %s, peer: %s): %s", projectID, peerID, err)
	}

	currentModel.Id = &peerResponse.ID

	// putting api keys into parameter store (needed for read operation)
	// the api keys might have been updated therefore we need to do this here
	_, err = putParameterIntoParameterStore(currentModel.Id, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus:      handler.InProgress,
		Message:              "Update Complete",
		ResourceModel:        currentModel,
		CallbackDelaySeconds: 10,
		CallbackContext: map[string]interface{}{
			"stateName": peerResponse.StatusName,
		},
	}, nil
}

// Delete handles the Delete event from the Cloudformation service.
func Delete(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	if _, ok := req.CallbackContext["stateName"]; ok {
		return validateProgress(client, currentModel, []string{"DELETED"})
	}

	projectId := *currentModel.ProjectId
	peerId := *currentModel.Id
	_, err = client.Peers.Delete(context.Background(), projectId, peerId)
	peeringDeleted := true
	if err != nil {
		peeringDeleted = false
	}

	_, err = deleteParameterFromParameterStore(currentModel.Id, req.Session)
	parameterDeleted := true
	if err != nil {
		parameterDeleted = false
	}

	if !peeringDeleted && !parameterDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete both peering and parameter from store for peering with id %s (awsId: %s)", *currentModel.Id, *currentModel.ConnectionId)
	}
	if !peeringDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete peering with id %s (awsId: %s)", *currentModel.Id, *currentModel.ConnectionId)
	}
	if !parameterDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete parameter for peering with id %s", *currentModel.Id)
	}

	return handler.ProgressEvent{
		OperationStatus:      handler.InProgress,
		Message:              "Delete Complete",
		ResourceModel:        currentModel,
		CallbackDelaySeconds: 10,
		CallbackContext: map[string]interface{}{
			"stateName": "DELETING",
		},
	}, nil
}

// List handles the List event from the Cloudformation service.
func List(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	projectID := *currentModel.ProjectId
	peerResponse, _, err := client.Peers.List(context.Background(), projectID, &matlasClient.ContainersListOptions{})
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error reading pf list peer with id(project: %s): %s", projectID, err)
	}

	var models []Model
	for _, peer := range peerResponse {
		var model Model
		model.AccepterRegionName = &peer.AccepterRegionName
		model.AwsAccountId = &peer.AWSAccountID
		model.RouteTableCidrBlock = &peer.RouteTableCIDRBlock
		model.VpcId = &peer.VpcID
		model.ConnectionId = &peer.ConnectionID
		model.ErrorStateName = &peer.ErrorStateName
		model.StatusName = &peer.StatusName
		model.ProviderName = &peer.ProviderName

		models = append(models, model)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "List Complete",
		ResourceModel:   models,
	}, nil
}

func validateProgress(client *matlasClient.Client, currentModel *Model, targetStates []string) (handler.ProgressEvent, error) {
	isReady, state, err := networkPeeringInTargetState(client, *currentModel.ProjectId, *currentModel.Id, targetStates)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	if !isReady {
		p := handler.NewProgressEvent()
		p.ResourceModel = currentModel
		p.OperationStatus = handler.InProgress
		p.CallbackDelaySeconds = 15
		p.Message = "Pending"
		p.CallbackContext = map[string]interface{}{
			"stateName": state,
		}
		return p, nil
	}

	p := handler.NewProgressEvent()
	p.ResourceModel = currentModel
	p.OperationStatus = handler.Success
	p.Message = "Complete"
	return p, nil
}

func networkPeeringInTargetState(client *matlasClient.Client, projectId string, peerId string, targetStates []string) (bool, string, error) {
	peerResponse, resp, err := client.Peers.Get(context.Background(), projectId, peerId)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 && stringInSlice("DELETED", targetStates) {
			return true, "DELETED", nil
		}
		return false, "ERROR", fmt.Errorf("error fetching network peering info (%s): %s", peerId, err)
	}
	errStatePointer := &peerResponse.ErrorStateName
	if errStatePointer != nil && (*errStatePointer == "REJECTED" || *errStatePointer == "EXPIRED" || *errStatePointer == "INVALID_ARGUMENT") {
		return false, "ERROR", fmt.Errorf("peering is in error state (%s): %s", peerId, *errStatePointer)
	}
	return stringInSlice(peerResponse.StatusName, targetStates), peerResponse.StatusName, nil
}

func stringInSlice(state string, targetStates []string) bool {
	for _, b := range targetStates {
		if b == state {
			return true
		}
	}
	return false
}

type ParameterToBePersistedSpec struct {
	ApiKeys   *ApiKeyDefinition
	ProjectId *string
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
		return nil, err
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
	networkPeeringParameterStorePrefix := "mongodbstpatlasv1networkpeering"
	return fmt.Sprintf("%s-%s", networkPeeringParameterStorePrefix, resourcePrimaryIdentifier)
}
