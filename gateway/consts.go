package gateway

import (
	jsoniter "github.com/json-iterator/go"
)

// VERSION of Sandwich-Producer, following Semantic Versioning.
const VERSION = "0.1"

var json = jsoniter.ConfigCompatibleWithStandardLibrary
var rediScripts = RediScripts{}
