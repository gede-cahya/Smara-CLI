package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListBundledSkills(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	require.NoError(t, os.Mkdir("skills", 0755))
	require.NoError(t, os.WriteFile(filepath.Join("skills", "planning-test.json"), []byte(`{
		"name":"planning-test",
		"description":"Test planning skill",
		"version":1,
		"tags":["planning"],
		"params":[{"name":"goal","type":"string","description":"Goal","required":true}],
		"category_path":["planning"],
		"steps":[{"tool":"planning_template","args":{"kind":"test-plan","goal":"__PARAM__goal"}}]
	}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join("skills", "bad.json"), []byte(`{bad`), 0644))

	items, err := ListBundledSkills()
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "planning-test", items[0].Name)
	assert.Equal(t, []string{"planning"}, items[0].Tags)
	assert.Equal(t, []string{"planning"}, items[0].CategoryPath)
	assert.Len(t, items[0].Params, 1)
}

func TestInstallBundledSkill(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Chdir(root)
	t.Setenv("HOME", home)
	require.NoError(t, os.Mkdir("skills", 0755))
	require.NoError(t, os.WriteFile(filepath.Join("skills", "planning-test.json"), []byte(`{
		"name":"planning-test",
		"description":"Test planning skill",
		"version":1,
		"tags":["planning"],
		"steps":[{"tool":"planning_template","args":{"kind":"test-plan","goal":"demo"}}]
	}`), 0644))

	sk, err := InstallBundledSkill("planning-test", "", false)
	require.NoError(t, err)
	assert.Equal(t, "planning-test", sk.Name)
	_, err = os.Stat(filepath.Join(home, ".smara", "skills", "planning-test.json"))
	assert.NoError(t, err)

	_, err = InstallBundledSkill("planning-test", "", false)
	assert.Error(t, err)

	aliased, err := InstallBundledSkill("planning-test", "planning-test-copy", false)
	require.NoError(t, err)
	assert.Equal(t, "planning-test-copy", aliased.Name)
}

func TestInstallBundledSkillRejectsTraversal(t *testing.T) {
	_, err := InstallBundledSkill("../secret", "", false)
	assert.Error(t, err)
}
