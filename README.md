# Casbin Go Cloud Development kit based Adapter
[![Go Reference](https://pkg.go.dev/badge/github.com/bartventer/casbin-go-cloud-adapter.svg)](https://pkg.go.dev/github.com/bartventer/casbin-go-cloud-adapter)
[![Go Report Card](https://goreportcard.com/badge/github.com/bartventer/casbin-go-cloud-adapter)](https://goreportcard.com/report/github.com/bartventer/casbin-go-cloud-adapter)
[![Coverage Status](https://coveralls.io/repos/github/bartventer/casbin-go-cloud-adapter/badge.svg?branch=master)](https://coveralls.io/github/bartventer/casbin-go-cloud-adapter?branch=master)
[![Build](https://github.com/bartventer/casbin-go-cloud-adapter/actions/workflows/go.yml/badge.svg)](https://github.com/bartventer/casbin-go-cloud-adapter/actions/workflows/go.yml)
[![Release](https://img.shields.io/github/release/bartventer/casbin-go-cloud-adapter.svg)](https://github.com/bartventer/casbin-go-cloud-adapter/releases/latest)

[Casbin](https://github.com/casbin/casbin) Adapter built on top of [gocloud.dev](https://gocloud.dev/).

## Installation

```
go get github.com/bartventer/casbin-go-cloud-adapter
```

## Usage

Configuration is slightly different for each provider as it needs to get different settings from environment. You can read more about URLs and configuration here: https://gocloud.dev/concepts/urls/.

Supported providers:
- [Google Cloud Firestore](https://cloud.google.com/firestore/)
- [Amazon DynamoDB](https://aws.amazon.com/dynamodb/)
- [Azure Cosmos DB](https://learn.microsoft.com/en-us/azure/cosmos-db/)
- [MongoDB](https://mongodb.org/)
- In-Memory Document Store (useful for local testing and single node installs)

You can view provider configuration examples here: https://github.com/google/go-cloud/tree/master/docstore.

### Google Cloud Firestore

Firestore URLs provide the project and collection, as well as the field that holds the document name (e.g. `firestore://projects/my-project/databases/(default)/documents/my-collection?name_field=userID`).

`casbin-go-cloud-adapter` will use Application Default Credentials; if you have authenticated via [gcloud auth application-default login](https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login), it will use those credentials. See [Application Default Credentials](https://cloud.google.com/docs/authentication#service-accounts) to learn about authentication alternatives, including using environment variables.

```go
import (
	"context"
	cloudadapter "github.com/bartventer/casbin-go-cloud-adapter"
	// Enable Firestore driver
	_ "github.com/bartventer/casbin-go-cloud-adapter/drivers/gcpfirestore"
	
	"github.com/casbin/casbin/v2"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url := "firestore://projects/casbin-project/databases/(default)/documents/casbin_rule?name_field=id"
	a, err := cloudadapter.New(ctx, url)
	if err != nil {
		panic(err)
	}

	e, err := casbin.NewEnforcer("examples/rbac_model.conf", a)
	if err != nil {
		panic(err)
	}

	// Load the policy from DB.
	e.LoadPolicy()

	// Check the permission.
	e.Enforce("alice", "data1", "read")

	// Modify the policy.
	// e.AddPolicy(...)
	// e.RemovePolicy(...)

	// Save the policy back to DB.
	e.SavePolicy()
}
```

### Amazon DynamoDB

DynamoDB URLs provide the table, partition key field and optionally the sort key field for the collection (e.g. `dynamodb://my-table?partition_key=name`).

`casbin-go-cloud-adapter` will create a default AWS Session with the SharedConfigEnable option enabled; if you have authenticated with the AWS CLI, it will use those credentials. See [AWS Session](https://docs.aws.amazon.com/sdk-for-go/api/aws/session/) to learn about authentication alternatives, including using environment variables.

```go
import (
	"context"
	cloudadapter "github.com/bartventer/casbin-go-cloud-adapter"
	// Enable DynamoDB driver
	_ "github.com/bartventer/casbin-go-cloud-adapter/drivers/awsdynamodb"
	
	"github.com/casbin/casbin/v2"
)	

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url := "dynamodb://casbin_test?partition_key=id"
	a, err := cloudadapter.New(ctx, url)
	if err != nil {
		panic(err)
	}

	e, err := casbin.NewEnforcer("examples/rbac_model.conf", a)
	if err != nil {
		panic(err)
	}

	// Load the policy from DB.
	e.LoadPolicy()

	// Check the permission.
	e.Enforce("alice", "data1", "read")

	// Modify the policy.
	// e.AddPolicy(...)
	// e.RemovePolicy(...)

	// Save the policy back to DB.
	e.SavePolicy()
}
```

### Azure Cosmos DB

Azure Cosmos DB is compatible with the MongoDB API. You can use the `mongodocstore` package to connect to Cosmos DB. You must create an Azure Cosmos account and get the MongoDB connection string.

When you use MongoDB URLs to connect to Cosmos DB, specify the Mongo server URL by setting the `MONGO_SERVER_URL` environment variable to the connection string. See the [MongoDB section](#mongodb) for more details and examples on how to use the package.


### MongoDB

MongoDB URLs provide the database and collection, and optionally the field that holds the document ID (e.g. `mongo://my-db/my-collection?id_field=userID`). Specify the Mongo server URL by setting the `MONGO_SERVER_URL` environment variable.

```go
import (
	"context"
	cloudadapter "github.com/bartventer/casbin-go-cloud-adapter"
	// Enable MongoDB driver
	_ "github.com/bartventer/casbin-go-cloud-adapter/drivers/mongodocstore"
	
	"github.com/casbin/casbin/v2"
)

func main() {
	// Set the MONGO_SERVER_URL environment variable to the MongoDB connection string.
	os.Setenv("MONGO_SERVER_URL", "mongodb://localhost:27017")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url := "mongo://casbin_test/casbin_rule?id_field=id"
	a, err := cloudadapter.New(ctx, url)
	if err != nil {
		panic(err)
	}

	e, err := casbin.NewEnforcer("examples/rbac_model.conf", a)
	if err != nil {
		panic(err)
	}

	// Load the policy from DB.
	e.LoadPolicy()

	// Check the permission.
	e.Enforce("alice", "data1", "read")

	// Modify the policy.
	// e.AddPolicy(...)
	// e.RemovePolicy(...)

	// Save the policy back to DB.
	e.SavePolicy()
}
```

### In Memory

URLs for the in-memory store have a mem: scheme. The URL host is used as the the collection name, and the URL path is used as the name of the document field to use as a primary key (e.g. `mem://collection/keyField`).

```go
import (
	"context"
	cloudadapter "github.com/bartventer/casbin-go-cloud-adapter"
	// Enable in-memory driver
	_ "github.com/bartventer/casbin-go-cloud-adapter/drivers/memdocstore"
	
	"github.com/casbin/casbin/v2"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url := "mem://casbin_rule/id"
	a, err := cloudadapter.New(ctx, url)
	if err != nil {
		panic(err)
	}

	e, err := casbin.NewEnforcer("examples/rbac_model.conf", a)
	if err != nil {
		panic(err)
	}

	// Load the policy from DB.
	e.LoadPolicy()

	// Check the permission.
	e.Enforce("alice", "data1", "read")

	// Modify the policy.
	// e.AddPolicy(...)
	// e.RemovePolicy(...)

	// Save the policy back to DB.
	e.SavePolicy()
}
```


## About Go Cloud Dev

Portable Cloud APIs in Go. Strives to implement these APIs for the leading Cloud providers: AWS, GCP and Azure, as well as provide a local (on-prem) implementation such as MongoDB, In-Memory, etc.

Using the Go CDK you can write your application code once using these idiomatic APIs, test locally using the local versions, and then deploy to a cloud provider with only minimal setup-time changes.

## Further Reading

- [Go CDK](https://gocloud.dev/): For more information on the Go CDK
- [Go CDK Docstore](https://gocloud.dev/howto/docstore/): For more information on the Go CDK Docstore package

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
