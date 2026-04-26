package cmd

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigSet_UnknownKey(t *testing.T) {
	_, ok := settableKeys["bogus.key"]
	if ok {
		t.Fatal("bogus.key should not be in settableKeys")
	}

	// Verify all declared keys have at least one valid value.
	for key, vals := range settableKeys {
		if len(vals) == 0 {
			t.Errorf("settableKeys[%q] has no valid values", key)
		}
	}
}

func TestConfigSet_InvalidValue(t *testing.T) {
	for key, allowed := range settableKeys {
		bad := "definitely-not-valid-xyz"
		found := false
		for _, v := range allowed {
			if v == bad {
				found = true
				break
			}
		}
		if found {
			t.Fatalf("test value %q unexpectedly valid for %s", bad, key)
		}
	}
}

func TestConfigSet_WalkOrCreateNodePath_ExistingPath(t *testing.T) {
	input := `version: 1
php:
  version: "8.4"
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatal(err)
	}

	node := walkOrCreateNodePath(&doc, []string{"php", "version"})
	if node == nil {
		t.Fatal("walkOrCreateNodePath returned nil for existing path")
	}
	if node.Value != "8.4" {
		t.Errorf("value = %q, want %q", node.Value, "8.4")
	}
}

func TestConfigSet_WalkOrCreateNodePath_MissingIntermediate(t *testing.T) {
	input := `version: 1
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatal(err)
	}

	node := walkOrCreateNodePath(&doc, []string{"node", "packageManager"})
	if node == nil {
		t.Fatal("walkOrCreateNodePath returned nil for missing intermediate")
	}

	// The node should be a newly created scalar.
	if node.Kind != yaml.ScalarNode {
		t.Errorf("kind = %d, want ScalarNode (%d)", node.Kind, yaml.ScalarNode)
	}

	// Verify the intermediate mapping was created.
	top := doc.Content[0]
	found := false
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "node" {
			if top.Content[i+1].Kind != yaml.MappingNode {
				t.Errorf("intermediate 'node' is not a MappingNode")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("intermediate 'node' key not created")
	}
}

func TestConfigSet_WalkOrCreateNodePath_NonDocument(t *testing.T) {
	// A bare scalar node (not a document) should return nil.
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "hello"}
	result := walkOrCreateNodePath(node, []string{"foo"})
	if result != nil {
		t.Error("expected nil for non-document node")
	}
}
