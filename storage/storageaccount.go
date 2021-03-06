package storage

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"log"

	"../iam"

	"github.com/Azure/azure-sdk-for-go/profiles/2018-03-01/storage/mgmt/storage"
	"github.com/Azure/azure-storage-blob-go/2016-05-31/azblob"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
)

const (
	errorPrefix = "Cannot create storage account, reason: %v"
)

func getStorageAccountKey(cntx context.Context, storageAccountsClient storage.AccountsClient, resourceGroupName, storageAccountName string) (key string, err error) {
	listKeys, err := storageAccountsClient.ListKeys(
		cntx,
		resourceGroupName,
		storageAccountName)
	if err != nil {
		return key, fmt.Errorf("cannot list storage account keys: %v", err)
	}
	storageAccountKeys := *listKeys.Keys
	key = *storageAccountKeys[0].Value
	return key, err
}

// UploadDataToContainer uploads data to a container
func UploadDataToContainer(cntx context.Context, containerURL azblob.ContainerURL, blobFileName, blobFileAddress string) (err error) {
	_, err = containerURL.Create(cntx, azblob.Metadata{}, azblob.PublicAccessNone)
	if err != nil {
		return fmt.Errorf("cannot create container: %v", err)
	}
	blobURL := containerURL.NewBlockBlobURL(blobFileName)
	file, err := os.Open(blobFileAddress)
	if err != nil {
		return fmt.Errorf("cannot read blob file: %v", err)
	}
	_, err = azblob.UploadFileToBlockBlob(cntx, file, blobURL, azblob.UploadToBlockBlobOptions{
		BlockSize:   4 * 1024 * 1024,
		Parallelism: 16})
	return err
}

// GetDataplaneURL returns dataplane URL
func GetDataplaneURL(cntx context.Context, storageAccountsClient storage.AccountsClient, storageEndpointSuffix, storageAccountName, resourceGroupName, storageContainerName string) (containerURL azblob.ContainerURL, err error) {
	storageAccountKey, err := getStorageAccountKey(cntx, storageAccountsClient, resourceGroupName, storageAccountName)
	if err != nil {
		return containerURL, fmt.Errorf("cannot get stroage account key: %v", err)
	}
	credential := azblob.NewSharedKeyCredential(storageAccountName, storageAccountKey)
	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	URL, err := url.Parse(fmt.Sprintf("https://%s.blob.%s/%s", storageAccountName, storageEndpointSuffix, storageContainerName))
	if err != nil {
		return containerURL, fmt.Errorf("cannot create container URL: %v", err)
	}
	containerURL = azblob.NewContainerURL(*URL, pipeline)
	return containerURL, err
}

// GetStorageAccountsClient creates a new storage account client
func GetStorageAccountsClient(tenantID, clientID, clientSecret, armEndpoint, certPath, subscriptionID string) storage.AccountsClient {
	token, err := iam.GetResourceManagementToken(tenantID, clientID, clientSecret, armEndpoint, certPath)
	if err != nil {
		log.Fatal(fmt.Sprintf(errorPrefix, fmt.Sprintf("Cannot generate token. Error details: %v.", err)))
	}
	storageAccountsClient := storage.NewAccountsClientWithBaseURI(armEndpoint, subscriptionID)
	storageAccountsClient.Authorizer = autorest.NewBearerAuthorizer(token)
	return storageAccountsClient
}

// CreateStorageAccount creates a new storage account.
func CreateStorageAccount(cntx context.Context, storageAccountsClient storage.AccountsClient, accountName, rgName, location string) (s storage.Account, err error) {
	result, err := storageAccountsClient.CheckNameAvailability(
		cntx,
		storage.AccountCheckNameAvailabilityParameters{
			Name: to.StringPtr(accountName),
			Type: to.StringPtr("Microsoft.Storage/storageAccounts"),
		})
	if err != nil {
		return s, fmt.Errorf(errorPrefix, err)
	}
	if *result.NameAvailable != true {
		return s, fmt.Errorf(errorPrefix, fmt.Sprintf("storage account name [%v] not available", accountName))
	}
	future, err := storageAccountsClient.Create(
		cntx,
		rgName,
		accountName,
		storage.AccountCreateParameters{
			Sku: &storage.Sku{
				Name: storage.StandardLRS},
			Location: to.StringPtr(location),
			AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{},
		})
	if err != nil {
		return s, fmt.Errorf(fmt.Sprintf(errorPrefix, err))
	}
	err = future.WaitForCompletion(cntx, storageAccountsClient.Client)
	if err != nil {
		return s, fmt.Errorf(fmt.Sprintf(errorPrefix, fmt.Sprintf("cannot get the storage account create future response: %v", err)))
	}
	return future.Result(storageAccountsClient)
}
