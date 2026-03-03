package promotion

// PromotionInstance represents a single promotion operation (one PR to create)
type PromotionInstance struct {
	Metadata          PromotionInstanceMetaData `deep:"-"`
	ComputedSyncPaths map[string]string         // key=target, value=source
}

// PromotionInstanceMetaData contains metadata about a promotion
type PromotionInstanceMetaData struct {
	SourcePath                     string
	TargetPaths                    []string
	TargetDescription              string
	PerComponentSkippedTargetPaths map[string][]string
	ComponentNames                 []string
	AutoMerge                      bool
	BlockList                      []string
}

// RelevantComponent identifies a source path + component that was touched by a change.
type RelevantComponent struct {
	SourcePath    string
	ComponentName string
	AutoMerge     bool
}
