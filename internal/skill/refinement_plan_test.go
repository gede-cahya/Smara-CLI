package skill

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffSkillsSummarizesStructuralChanges(t *testing.T) {
	oldSkill := &Skill{
		Name:         "deploy-api",
		Description:  "Deploy API service",
		Steps:        []Step{{Tool: "shell", Args: map[string]interface{}{"cmd": "build"}}, {Tool: "notify"}},
		Params:       []ParamDef{{Name: "env", Type: "string"}},
		Dependencies: []string{"login"},
	}
	newSkill := &Skill{
		Name:         "deploy-api-v2",
		Description:  "Deploy API service safely",
		Steps:        []Step{{Tool: "shell", Args: map[string]interface{}{"cmd": "test"}}, {Tool: "notify"}, {Tool: "audit"}},
		Params:       []ParamDef{{Name: "region", Type: "string"}},
		Dependencies: []string{"backup"},
	}

	diff := DiffSkills(oldSkill, newSkill)

	assert.True(t, diff.NameChanged)
	assert.True(t, diff.DescriptionChanged)
	assert.Equal(t, []int{0}, diff.StepsChanged)
	assert.Equal(t, []int{2}, diff.StepsAdded)
	assert.Equal(t, []string{"region"}, diff.ParamsAdded)
	assert.Equal(t, []string{"env"}, diff.ParamsRemoved)
	assert.Equal(t, []string{"backup"}, diff.DependenciesAdded)
	assert.Equal(t, []string{"login"}, diff.DependenciesRemoved)
}

func TestBuildRefinementPreviewRunsLint(t *testing.T) {
	original := &Skill{Name: "deploy-api", Description: "Deploy API service", Steps: []Step{{Tool: "shell"}}}
	proposed := &Skill{Name: "deploy-api", Description: "Deploy API service with checks", Steps: []Step{{Tool: "unknown"}}}

	preview := BuildRefinementPreview(original, proposed, LintOptions{KnownTools: map[string]bool{"shell": true}})

	assert.Equal(t, "deploy-api", preview.OriginalName)
	assert.Equal(t, "deploy-api", preview.ProposedName)
	require.Len(t, preview.Lint.Issues, 1)
	assert.Equal(t, "steps[0].tool", preview.Lint.Issues[0].Field)
	assert.Contains(t, preview.Summary, "description changed")

	data, err := preview.ToJSON()
	require.NoError(t, err)
	assert.Contains(t, string(data), "unknown tool")
}

func TestApplyRefinementWithLintNormalizesNameAndBumpsVersion(t *testing.T) {
	setupTestHome(t)
	prior := &Skill{Name: "deploy-api", Description: "Deploy API service", Version: 2, Steps: []Step{{Tool: "shell"}}}
	require.NoError(t, Save(prior, nil))

	proposal := &Skill{Name: "renamed-skill", Description: "Deploy API service with checks", Version: 1, Steps: []Step{{Tool: "shell", Args: map[string]interface{}{"cmd": "test"}}}}
	proposalJSON, err := json.Marshal(proposal)
	require.NoError(t, err)

	applied, preview, err := ApplyRefinementWithLint("deploy-api", proposalJSON, nil, "manual", LintOptions{KnownTools: map[string]bool{"shell": true}}, false)

	require.NoError(t, err)
	assert.Equal(t, "deploy-api", applied.Name)
	assert.Equal(t, "deploy-api", preview.ProposedName)
	assert.False(t, preview.Diff.NameChanged)
	assert.Equal(t, 3, applied.Version)
	require.Len(t, applied.Lineage, 1)
	assert.Equal(t, 2, applied.Lineage[0].Version)
}

func TestApplyRefinementWithLintRejectsInvalidProposal(t *testing.T) {
	setupTestHome(t)
	require.NoError(t, Save(&Skill{Name: "deploy-api", Description: "Deploy API service", Version: 1, Steps: []Step{{Tool: "shell"}}}, nil))

	proposal := &Skill{Name: "deploy-api", Description: "Deploy API service with checks", Version: 2, Steps: []Step{{Tool: "unknown"}}}
	proposalJSON, err := json.Marshal(proposal)
	require.NoError(t, err)

	applied, preview, err := ApplyRefinementWithLint("deploy-api", proposalJSON, nil, "manual", LintOptions{KnownTools: map[string]bool{"shell": true}}, false)

	assert.Nil(t, applied)
	require.Error(t, err)
	assert.True(t, preview.Lint.HasErrors())
}
