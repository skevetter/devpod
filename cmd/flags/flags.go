package flags

import (
	"os"
	"strconv"

	"github.com/skevetter/log"
	"github.com/skevetter/devpod/pkg/platform"
	flag "github.com/spf13/pflag"
)

type GlobalFlags struct {
	Context    string
	Provider   string
	AgentDir   string
	DevPodHome string
	UID        string
	Owner      platform.OwnerFilter

	LogOutput string
	Debug     bool
	Silent    bool
}

// Defines a string flag with specified name, environment variable, default value, and usage string.
// The argument p points to a string variable in which to store the value of the flag.
func StringVarE(f *flag.FlagSet, p *string, name string, env string, value string, usage string) {
	key := "DEVPOD_" + env
	f.StringVar(p, name, GetEnv(key, value), usage+". You can also use "+key+" to set this")
}

func GetEnv(key string, def string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return def
}

// Defines a bool flag with specified name, environment variable, default value, and usage string.
// The argument p points to a bool variable in which to store the value of the flag.
func BoolVarE(f *flag.FlagSet, p *bool, name string, env string, value bool, usage string) {
	key := "DEVPOD_" + env
	f.BoolVar(p, name, GetBoolEnv(key, value), usage+". You can also use "+key+" to set this")
}

func GetBoolEnv(key string, def bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		result, err := strconv.ParseBool(value)
		if err == nil {
			return result
		}
		log.Default.Errorf("error parsing bool value: %v", err)
		return def
	}
	return def
}

// SetGlobalFlags applies the global flags
func SetGlobalFlags(flags *flag.FlagSet) *GlobalFlags {
	globalFlags := &GlobalFlags{}

	flags.StringVar(&globalFlags.DevPodHome, "devpod-home", "", "If defined will override the default devpod home. You can also use DEVPOD_HOME to set this")
	StringVarE(flags, &globalFlags.LogOutput, "log-output", "LOG_OUTPUT", "plain", "The log format to use. Can be either plain, raw or json")
	StringVarE(flags, &globalFlags.Context, "context", "CONTEXT", "", "The context to use")
	StringVarE(flags, &globalFlags.Provider, "provider", "PROVIDER", "", "The provider to use. Needs to be configured for the selected context")
	BoolVarE(flags, &globalFlags.Debug, "debug", "DEBUG", false, "Prints the stack trace if an error occurs")
	BoolVarE(flags, &globalFlags.Silent, "silent", "SILENT", false, "Run in silent mode and prevents any devpod log output except panics & fatals")

	flags.Var(&globalFlags.Owner, "owner", "Show pro workspaces for owner")
	_ = flags.MarkHidden("owner")
	flags.StringVar(&globalFlags.UID, "uid", "", "Set UID for workspace")
	_ = flags.MarkHidden("uid")
	flags.StringVar(&globalFlags.AgentDir, "agent-dir", "", "The data folder where agent data is stored.")
	_ = flags.MarkHidden("agent-dir")
	return globalFlags
}
