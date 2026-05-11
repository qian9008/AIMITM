package rewriter

import (
	"airoxy-linux/internal/config"
	"github.com/tidwall/sjson"
)

func RewriteJSON(data []byte, rules []config.RewriteRule) ([]byte, error) {
	var err error
	result := data

	for _, rule := range rules {
		switch rule.Action {
		case "REPLACE", "ADD":
			result, err = sjson.SetBytes(result, rule.Path, rule.Value)
		case "REMOVE":
			result, err = sjson.DeleteBytes(result, rule.Path)
		}
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}
