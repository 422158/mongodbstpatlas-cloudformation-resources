package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/cluster/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/spf13/cast"
	"go.mongodb.org/atlas/mongodbatlas"
)

func castNO64(i *int64) *int {
	x := cast.ToInt(&i)
	return &x
}
func cast64(i *int) *int64 {
	x := cast.ToInt64(&i)
	return &x
}

// Create handles the Create event from the Cloudformation service.
func Create(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	if _, ok := req.CallbackContext["stateName"]; ok {
		return validateProgress(client, req, currentModel, "IDLE", "CREATING")
	}

	projectID := *currentModel.ProjectId

	if len(currentModel.ReplicationSpecs) > 0 {
		if currentModel.ClusterType != nil {
			return handler.ProgressEvent{}, errors.New("error creating cluster: ClusterType should be set when `ReplicationSpecs` is set")
		}

		if currentModel.NumShards != nil {
			return handler.ProgressEvent{}, errors.New("error creating cluster: NumShards should be set when `ReplicationSpecs` is set")
		}
	}

	var autoScaling *mongodbatlas.AutoScaling
	if currentModel.AutoScaling != nil {
		autoScaling = &mongodbatlas.AutoScaling{
			DiskGBEnabled: currentModel.AutoScaling.DiskGBEnabled,
		}
		if currentModel.AutoScaling.Compute != nil {
			compute := &mongodbatlas.Compute{}
			if currentModel.AutoScaling.Compute.Enabled != nil {
				compute.Enabled = currentModel.AutoScaling.Compute.Enabled
			}
			if currentModel.AutoScaling.Compute.ScaleDownEnabled != nil {
				compute.ScaleDownEnabled = currentModel.AutoScaling.Compute.ScaleDownEnabled
			}

			autoScaling.Compute = compute
		}
	}

	clusterRequest := &mongodbatlas.Cluster{
		Name:                     cast.ToString(currentModel.Name),
		EncryptionAtRestProvider: cast.ToString(currentModel.EncryptionAtRestProvider),
		ClusterType:              cast.ToString(currentModel.ClusterType),
		AutoScaling:              autoScaling,
		NumShards:                cast64(currentModel.NumShards),
	}

	if currentModel.BackupEnabled != nil {
		clusterRequest.BackupEnabled = currentModel.BackupEnabled
	}

	if currentModel.ProviderBackupEnabled != nil {
		clusterRequest.ProviderBackupEnabled = currentModel.ProviderBackupEnabled
	}

	if currentModel.DiskSizeGB != nil {
		clusterRequest.DiskSizeGB = currentModel.DiskSizeGB
	}

	if currentModel.MongoDBMajorVersion != nil {
		clusterRequest.MongoDBMajorVersion = formatMongoDBMajorVersion(*currentModel.MongoDBMajorVersion)
	}

	if currentModel.BiConnector != nil {
		clusterRequest.BiConnector = expandBiConnector(currentModel.BiConnector)
	}

	if currentModel.ProviderSettings != nil {
		clusterRequest.ProviderSettings = expandProviderSettings(currentModel.ProviderSettings)
	}

	if currentModel.ReplicationSpecs != nil {
		clusterRequest.ReplicationSpecs = expandReplicationSpecs(currentModel.ReplicationSpecs)
	}

	cluster, resp, err := client.Clusters.Create(context.Background(), projectID, clusterRequest)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error creating cluster: %w %v", err, &resp)
	}

	currentModel.Id = &cluster.ID
	currentModel.StateName = &cluster.StateName

	cfnid := buildClusterCfnIdentifier(currentModel.ProjectId, currentModel.Name)

	currentModel.ClusterCfnIdentifier = &cfnid

	// putting required parameters into parameter store (needed for read operation)
	_, err = putParameterIntoParameterStore(currentModel.ClusterCfnIdentifier, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId, ClusterName: currentModel.Name}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus:      handler.InProgress,
		Message:              fmt.Sprintf("Create Cluster `%s`", cluster.StateName),
		ResourceModel:        currentModel,
		CallbackDelaySeconds: 65,
		CallbackContext: map[string]interface{}{
			"stateName": cluster.StateName,
		},
	}, nil
}

// Read handles the Read event from the Cloudformation service.
func Read(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	params, err := getParameterFromParameterStore(currentModel.ClusterCfnIdentifier, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	client, err := util.CreateMongoDBClient(*params.ApiKeys.PublicKey, *params.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	cluster, _, err := client.Clusters.Get(context.Background(), *params.ProjectId, *params.ClusterName)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error fetching cluster info (%s): %s", *params.ClusterName, err)
	}

	currentModel.AutoScaling = &AutoScaling{
		DiskGBEnabled: cluster.AutoScaling.DiskGBEnabled,
		Compute: &AutoScalingCompute{
			ScaleDownEnabled: cluster.AutoScaling.Compute.ScaleDownEnabled,
			Enabled:          cluster.AutoScaling.Compute.Enabled,
		},
	}

	currentModel.BackupEnabled = cluster.BackupEnabled

	currentModel.BiConnector = &BiConnector{
		ReadPreference: &cluster.BiConnector.ReadPreference,
		Enabled:        cluster.BiConnector.Enabled,
	}

	currentModel.ProviderBackupEnabled = cluster.ProviderBackupEnabled
	currentModel.ClusterType = &cluster.ClusterType
	currentModel.DiskSizeGB = cluster.DiskSizeGB
	currentModel.EncryptionAtRestProvider = &cluster.EncryptionAtRestProvider
	currentModel.MongoDBMajorVersion = &cluster.MongoDBVersion

	if cluster.NumShards != nil {
		currentModel.NumShards = castNO64(cluster.NumShards)
	}

	currentModel.MongoDBVersion = &cluster.MongoDBVersion
	currentModel.MongoURI = &cluster.MongoURI
	currentModel.MongoURIUpdated = &cluster.MongoURIUpdated
	currentModel.MongoURIWithOptions = &cluster.MongoURIWithOptions
	currentModel.Paused = cluster.Paused
	currentModel.SrvAddress = &cluster.SrvAddress
	currentModel.StateName = &cluster.StateName

	if &cluster.ConnectionStrings.PrivateSrv != nil {
		currentModel.ConnectionString = &cluster.ConnectionStrings.Standard
	}
	if &cluster.ConnectionStrings.StandardSrv != nil {
		currentModel.SrvConnectionString = &cluster.ConnectionStrings.StandardSrv
	}

	currentModel.BiConnector = &BiConnector{
		ReadPreference: &cluster.BiConnector.ReadPreference,
		Enabled:        cluster.BiConnector.Enabled,
	}

	currentModel.Id = &cluster.ID

	if cluster.ProviderSettings != nil {
		currentModel.ProviderSettings = &ProviderSettings{
			BackingProviderName: &cluster.ProviderSettings.BackingProviderName,
			ProviderName:        &cluster.ProviderSettings.ProviderName,
			AutoScaling: &AutoScalingProvider{
				Compute: &AutoScalingProviderCompute{
					MinInstanceSize: &cluster.ProviderSettings.AutoScaling.Compute.MinInstanceSize,
					MaxInstanceSize: &cluster.ProviderSettings.AutoScaling.Compute.MaxInstanceSize,
				},
			},
			InstanceSizeName: &cluster.ProviderSettings.InstanceSizeName,
			RegionName:       &cluster.ProviderSettings.RegionName,
		}
	}

	currentModel.ReplicationSpecs = flattenReplicationSpecs(cluster.ReplicationSpecs)

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
		return validateProgress(client, req, currentModel, "IDLE", "UPDATING")
	}

	projectID := *currentModel.ProjectId
	clusterName := *currentModel.Name

	if len(currentModel.ReplicationSpecs) > 0 {
		if currentModel.ClusterType != nil {
			return handler.ProgressEvent{}, errors.New("error updating cluster: ClusterType should be set when `ReplicationSpecs` is set")
		}

		if currentModel.NumShards != nil {
			return handler.ProgressEvent{}, errors.New("error updating cluster: NumShards should be set when `ReplicationSpecs` is set")
		}
	}

	var autoScaling *mongodbatlas.AutoScaling
	if currentModel.AutoScaling != nil {
		autoScaling = &mongodbatlas.AutoScaling{
			DiskGBEnabled: currentModel.AutoScaling.DiskGBEnabled,
		}
		if currentModel.AutoScaling.Compute != nil {
			compute := &mongodbatlas.Compute{}
			if currentModel.AutoScaling.Compute.Enabled != nil {
				compute.Enabled = currentModel.AutoScaling.Compute.Enabled
			}
			if currentModel.AutoScaling.Compute.ScaleDownEnabled != nil {
				compute.ScaleDownEnabled = currentModel.AutoScaling.Compute.ScaleDownEnabled
			}

			autoScaling.Compute = compute
		}
	}

	clusterRequest := &mongodbatlas.Cluster{
		Name:                     cast.ToString(currentModel.Name),
		EncryptionAtRestProvider: cast.ToString(currentModel.EncryptionAtRestProvider),
		ClusterType:              cast.ToString(currentModel.ClusterType),
		AutoScaling:              autoScaling,
		NumShards:                cast64(currentModel.NumShards),
	}

	if currentModel.BackupEnabled != nil {
		clusterRequest.BackupEnabled = currentModel.BackupEnabled
	}

	if currentModel.ProviderBackupEnabled != nil {
		clusterRequest.ProviderBackupEnabled = currentModel.ProviderBackupEnabled
	}

	if currentModel.DiskSizeGB != nil {
		clusterRequest.DiskSizeGB = currentModel.DiskSizeGB
	}

	if currentModel.MongoDBMajorVersion != nil {
		clusterRequest.MongoDBMajorVersion = formatMongoDBMajorVersion(*currentModel.MongoDBMajorVersion)
	}

	if currentModel.BiConnector != nil {
		clusterRequest.BiConnector = expandBiConnector(currentModel.BiConnector)
	}

	if currentModel.ProviderSettings != nil {
		clusterRequest.ProviderSettings = expandProviderSettings(currentModel.ProviderSettings)
	}

	if currentModel.ReplicationSpecs != nil {
		clusterRequest.ReplicationSpecs = expandReplicationSpecs(currentModel.ReplicationSpecs)
	}

	cluster, _, err := client.Clusters.Update(context.Background(), projectID, clusterName, clusterRequest)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error creating cluster: %s", err)
	}

	currentModel.Id = &cluster.ID

	cfnid := buildClusterCfnIdentifier(currentModel.ProjectId, currentModel.Name)

	currentModel.ClusterCfnIdentifier = &cfnid

	// putting required parameter into parameter store
	// the api keys might have been updated therefore we need to do this here
	_, err = putParameterIntoParameterStore(currentModel.ClusterCfnIdentifier, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId, ClusterName: currentModel.Name}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus:      handler.InProgress,
		Message:              fmt.Sprintf("Update Cluster `%s`", cluster.StateName),
		ResourceModel:        currentModel,
		CallbackDelaySeconds: 65,
		CallbackContext: map[string]interface{}{
			"stateName": cluster.StateName,
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
		return validateProgress(client, req, currentModel, "DELETED", "DELETING")
	}

	projectID := *currentModel.ProjectId
	clusterName := *currentModel.Name

	_, err = client.Clusters.Delete(context.Background(), projectID, clusterName)
	if err != nil {
		// even when error occurs when deleting, we still want to delete parameter from parameter store
		_, errParams := deleteParameterFromParameterStore(currentModel.ClusterCfnIdentifier, req.Session)
		if errParams != nil {
			return handler.ProgressEvent{
				OperationStatus:  handler.Failed,
				Message:          "Delete Failed",
				HandlerErrorCode: "GeneralServiceException",
			}, fmt.Errorf("Error deleting cluster with name(%s): %s.\nError deleting api keys from parameter store: %s", clusterName, err, errParams)
		}
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("error deleting cluster with name (%s): %s", clusterName, err)
	}

	_, err = deleteParameterFromParameterStore(currentModel.ClusterCfnIdentifier, req.Session)
	if err != nil {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("error deleting parameters for cluster %s: %s", clusterName, err)
	}

	return handler.ProgressEvent{
		OperationStatus:      handler.InProgress,
		Message:              "Delete In Progress",
		ResourceModel:        currentModel,
		CallbackDelaySeconds: 65,
		CallbackContext: map[string]interface{}{
			"stateName": "DELETING",
		},
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

func expandBiConnector(biConnector *BiConnector) *mongodbatlas.BiConnector {
	return &mongodbatlas.BiConnector{
		Enabled:        biConnector.Enabled,
		ReadPreference: cast.ToString(biConnector.ReadPreference),
	}
}

func expandProviderSettings(providerSettings *ProviderSettings) *mongodbatlas.ProviderSettings {
	ps := &mongodbatlas.ProviderSettings{
		EncryptEBSVolume:    providerSettings.EncryptEBSVolume,
		RegionName:          cast.ToString(providerSettings.RegionName),
		BackingProviderName: cast.ToString(providerSettings.BackingProviderName),
		InstanceSizeName:    cast.ToString(providerSettings.InstanceSizeName),
		ProviderName:        cast.ToString(providerSettings.ProviderName),
		VolumeType:          cast.ToString(providerSettings.VolumeType),
	}
	if providerSettings.AutoScaling != nil {
		if providerSettings.AutoScaling.Compute != nil {
			ps.AutoScaling = &mongodbatlas.AutoScaling{Compute: &mongodbatlas.Compute{}}
			if providerSettings.AutoScaling.Compute.MaxInstanceSize != nil {
				ps.AutoScaling.Compute.MaxInstanceSize = *providerSettings.AutoScaling.Compute.MaxInstanceSize
			}
			if providerSettings.AutoScaling.Compute.MinInstanceSize != nil {
				ps.AutoScaling.Compute.MinInstanceSize = *providerSettings.AutoScaling.Compute.MinInstanceSize
			}
		}
	}
	if providerSettings.DiskIOPS != nil {
		ps.DiskIOPS = cast64(providerSettings.DiskIOPS)
	}
	return ps

}

func expandReplicationSpecs(replicationSpecs []ReplicationSpec) []mongodbatlas.ReplicationSpec {
	rSpecs := make([]mongodbatlas.ReplicationSpec, 0)

	for _, s := range replicationSpecs {
		rSpec := mongodbatlas.ReplicationSpec{
			ID:            cast.ToString(s.ID),
			NumShards:     cast64(s.NumShards),
			ZoneName:      cast.ToString(s.ZoneName),
			RegionsConfig: expandRegionsConfig(s.RegionsConfig),
		}

		rSpecs = append(rSpecs, rSpec)
	}
	return rSpecs
}

func expandRegionsConfig(regions []RegionConfig) map[string]mongodbatlas.RegionsConfig {
	regionsConfig := make(map[string]mongodbatlas.RegionsConfig)
	for _, region := range regions {
		regionsConfig[*region.RegionName] = mongodbatlas.RegionsConfig{
			AnalyticsNodes: cast64(region.AnalyticsNodes),
			ElectableNodes: cast64(region.ElectableNodes),
			Priority:       cast64(region.Priority),
			ReadOnlyNodes:  cast64(region.ReadOnlyNodes),
		}
	}
	return regionsConfig
}

func formatMongoDBMajorVersion(val interface{}) string {
	if strings.Contains(val.(string), ".") {
		return val.(string)
	}
	return fmt.Sprintf("%.1f", cast.ToFloat32(val))
}

func flattenReplicationSpecs(rSpecs []mongodbatlas.ReplicationSpec) []ReplicationSpec {
	specs := make([]ReplicationSpec, 0)
	for _, rSpec := range rSpecs {
		spec := ReplicationSpec{
			ID:            &rSpec.ID,
			NumShards:     castNO64(rSpec.NumShards),
			ZoneName:      &rSpec.ZoneName,
			RegionsConfig: flattenRegionsConfig(rSpec.RegionsConfig),
		}
		specs = append(specs, spec)
	}
	return specs
}

func flattenRegionsConfig(regionsConfig map[string]mongodbatlas.RegionsConfig) []RegionConfig {
	regions := make([]RegionConfig, 0)

	for regionName, regionConfig := range regionsConfig {
		region := RegionConfig{
			RegionName:     &regionName,
			Priority:       castNO64(regionConfig.Priority),
			AnalyticsNodes: castNO64(regionConfig.AnalyticsNodes),
			ElectableNodes: castNO64(regionConfig.ElectableNodes),
			ReadOnlyNodes:  castNO64(regionConfig.ReadOnlyNodes),
		}
		regions = append(regions, region)
	}
	return regions
}

func validateProgress(client *mongodbatlas.Client, req handler.Request, currentModel *Model, targetState string, pendingState string) (handler.ProgressEvent, error) {
	isReady, state, err := isClusterInTargetState(client, *currentModel.ProjectId, *currentModel.Name, targetState)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	if !isReady {
		p := handler.NewProgressEvent()
		p.ResourceModel = currentModel
		p.OperationStatus = handler.InProgress
		p.CallbackDelaySeconds = 60
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

func isClusterInTargetState(client *mongodbatlas.Client, projectID, clusterName, targetState string) (bool, string, error) {
	cluster, resp, err := client.Clusters.Get(context.Background(), projectID, clusterName)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return "DELETED" == targetState, "DELETED", nil
		}
		return false, "ERROR", fmt.Errorf("error fetching cluster info (%s): %s", clusterName, err)
	}
	return cluster.StateName == targetState, cluster.StateName, nil
}

type ParameterToBePersistedSpec struct {
	ApiKeys     *ApiKeyDefinition
	ProjectId   *string
	ClusterName *string
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

func buildClusterCfnIdentifier(projectId *string, clusterName *string) string {
	return fmt.Sprintf("%s-%s", *projectId, *clusterName)
}

func buildApiKeyParameterName(resourcePrimaryIdentifier string) string {
	// this is strictly coupled with permissions for handlers, changing this means changing permissions in handler
	// moreover changing this might cause polution in parameter store -  be sure you know what you are doing
	parameterStorePrefix := "mongodbstpatlasv1cluster"
	return fmt.Sprintf("%s-%s", parameterStorePrefix, resourcePrimaryIdentifier)
}
