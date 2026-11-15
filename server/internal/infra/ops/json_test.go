package ops

import "encoding/json"

func jsonUnmarshal(content []byte, target any) error { return json.Unmarshal(content, target) }
