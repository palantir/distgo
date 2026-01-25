package distgotaskprovider

import "github.com/palantir/godel/v2/framework/godellauncher"

// TaskInfo is the information needed to create a godel task.
// Based on the godel definition at https://github.com/palantir/godel/blob/8537d0ea9067d3bdd36d5db06069b71fde92188b/framework/pluginapi/v2/pluginapi/taskinfo.go#L44.
type TaskInfo struct {
	NameVar              string             `json:"name"`
	DescriptionVar       string             `json:"description"`
	CommandVar           []string           `json:"command"`
	GlobalFlagOptionsVar *GlobalFlagOptions `json:"globalFlagOptions"`
	VerifyOptionsVar     *VerifyOptions     `json:"verifyOptions"`

	// RegisterAsTopLevelDistgoTaskCommand indicates whether this task should be registered as a top-level command under
	// the "distgo-task" task. The command is always registered as a fully qualified command regardless of this value.
	// Even if this value is true, the command may not be registered as a top-level command if its name conflicts with
	// any default values or other with top-level commands registered by other assets.
	RegisterAsTopLevelDistgoTaskCommand bool `json:"registerAsTopLevelDistgoTaskCommand"`
}

// GlobalFlagOptions are the options for global flags on godel tasks.
// Based on the godel definition at https://github.com/palantir/godel/blob/8537d0ea9067d3bdd36d5db06069b71fde92188b/framework/pluginapi/v2/pluginapi/globalflagopts.go#L34
type GlobalFlagOptions struct {
	DebugFlagVar       string `json:"debugFlag"`
	ProjectDirFlagVar  string `json:"projectDirFlag"`
	//GodelConfigFlagVar string `json:"godelConfigFlag"`
	//ConfigFlagVar      string `json:"configFlag"`
}

// VerifyOptions are the options for verify options on godel tasks.
// Mirrors the godel definition at https://github.com/palantir/godel/blob/8537d0ea9067d3bdd36d5db06069b71fde92188b/framework/pluginapi/v2/pluginapi/verifyopts.go#L35
type VerifyOptions struct {
	VerifyTaskFlagsVar []VerifyFlag `json:"verifyTaskFlags"`
	OrderingVar        *int         `json:"ordering"`
	ApplyTrueArgsVar   []string     `json:"applyTrueArgs"`
	ApplyFalseArgsVar  []string     `json:"applyFalseArgs"`
}

// VerifyFlag specifies the settings for the verify flag for godel tasks.
// Mirrors the godel definition at https://github.com/palantir/godel/blob/429e630ed3d426c324ab6929ceb11f9aca553669/framework/pluginapi/verifyopts.go#L143
type VerifyFlag struct {
	NameVar        string                 `json:"name"`
	DescriptionVar string                 `json:"description"`
	TypeVar        godellauncher.FlagType `json:"type"`
}