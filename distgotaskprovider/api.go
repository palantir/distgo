package distgotaskprovider

import (
	"github.com/palantir/godel/v2/framework/godellauncher"
)

// TaskInfo is the information needed to create a distgo task.
// Based on the godel definition at https://github.com/palantir/godel/blob/8537d0ea9067d3bdd36d5db06069b71fde92188b/framework/pluginapi/v2/pluginapi/taskinfo.go#L44.
type TaskInfo struct {
	// Name is the name of the task. This is the task/command name registered by the distgo TaskProvider API.
	Name string `json:"name"`
	// Description is the description of the task. It is used as the "Short" description of the task/command.
	Description string `json:"description"`
	// Command specifies the arguments used to invoke the task/command on the asset. In many instances, this may be the
	// same value as Name, but it may be different if the task name for the purposes of the TaskProvider API is
	// different from the command used to invoke the task on the asset.
	Command []string `json:"command"`
	//GlobalFlagOptionsVar *GlobalFlagOptions `json:"globalFlagOptions"`
	VerifyOptions *VerifyOptions `json:"verifyOptions"`

	// RegisterAsTopLevelDistgoTaskCommand indicates whether this task should be registered as a top-level command under
	// the "distgo-task" task. The command is always registered as a fully qualified command regardless of this value.
	// Even if this value is true, the command may not be registered as a top-level command if its name conflicts with
	// any default values or other with top-level commands registered by other assets.
	RegisterAsTopLevelDistgoTaskCommand bool `json:"registerAsTopLevelDistgoTaskCommand"`
}

//// GlobalFlagOptions are the options for global flags on godel tasks.
//// Based on the godel definition at https://github.com/palantir/godel/blob/8537d0ea9067d3bdd36d5db06069b71fde92188b/framework/pluginapi/v2/pluginapi/globalflagopts.go#L34
//type GlobalFlagOptions struct {
//	DebugFlagVar      string `json:"debugFlag"`
//	ProjectDirFlagVar string `json:"projectDirFlag"`
//	//GodelConfigFlagVar string `json:"godelConfigFlag"`
//	//ConfigFlagVar      string `json:"configFlag"`
//}

// VerifyOptions specifies how the task should be run in "verify" mode.
// Based on the godel definition at https://github.com/palantir/godel/blob/8537d0ea9067d3bdd36d5db06069b71fde92188b/framework/pluginapi/v2/pluginapi/verifyopts.go#L35
type VerifyOptions struct {
	VerifyTaskFlags []VerifyFlag `json:"verifyTaskFlags"`
	//Ordering          *int     `json:"ordering"`
	ApplyTrueArgs  []string `json:"applyTrueArgs"`
	ApplyFalseArgs []string `json:"applyFalseArgs"`
}

// VerifyFlag specifies the settings for the verify flag for distgo TaskProvider tasks.
// Based on the godel definition at https://github.com/palantir/godel/blob/429e630ed3d426c324ab6929ceb11f9aca553669/framework/pluginapi/verifyopts.go#L143
type VerifyFlag struct {
	NameVar        string                 `json:"name"`
	DescriptionVar string                 `json:"description"`
	TypeVar        godellauncher.FlagType `json:"type"`
}
