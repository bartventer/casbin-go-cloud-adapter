// Package mongodocstore registers the [mongodocstore] driver with the docstore package.
package mongodocstore

import (
	// Import the docstore package to register the mongodocstore driver.
	_ "gocloud.dev/docstore/mongodocstore"
)
