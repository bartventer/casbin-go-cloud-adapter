/*
Package adapter provides a Casbin adapter built on top of gocloud.dev.
It supports multiple providers including Google Cloud Firestore, Amazon DynamoDB,
Azure Cosmos DB, MongoDB, and an In-Memory Document Store.

The adapter allows you to write your application code once using idiomatic APIs,
test locally using the local versions, and then deploy to a cloud provider with only minimal setup-time changes.

For more information on the Go CDK and the Go CDK Docstore package, visit:
- Go CDK: https://gocloud.dev/
- Go CDK Docstore: https://gocloud.dev/howto/docstore/
*/
package adapter
