package fixturize

import "github.com/boringsql/fixturize/masking"

const MasksFileName = masking.MasksFileName

type (
	SharedMasksFile = masking.SharedMasksFile
	SharedDatabase  = masking.SharedDatabase
	SharedColumn    = masking.SharedColumn
	SharedPolicy    = masking.SharedPolicy
)

func LoadSharedMasks(path string) (*SharedMasksFile, error) { return masking.LoadSharedMasks(path) }
func DiscoverMasksFile(startDir string) (string, error)     { return masking.DiscoverMasksFile(startDir) }
