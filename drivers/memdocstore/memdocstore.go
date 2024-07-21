// Package memdocstore registers the [memdocstore] driver with the docstore package.
package memdocstore

import (
	// Import the docstore package to register the memdocstore driver.
	_ "gocloud.dev/docstore/memdocstore"
)
