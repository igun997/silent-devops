package devopsv1

// DefaultLimits returns protocol hard ceilings. Deployments may negotiate lower
// values, never higher values without a protocol revision.
func DefaultLimits() *ProtocolLimits {
	return &ProtocolLimits{
		MaxMessageBytes:      4 << 20,
		MaxOutputBytes:       1 << 20,
		MaxOutputChunkBytes:  64 << 10,
		MaxMetrics:           4096,
		MaxLabelsPerMetric:   16,
		MaxCapabilities:      128,
		MaxQueueDepth:        256,
		MaxReasonBytes:       2048,
		MaxCommandBytes:      64 << 10,
		MaxUnitBytes:         256,
		MaxPathBytes:         4096,
		MaxJobTimeoutSeconds: 3600,
		MaxSshTtlSeconds:     3600,
		MaxPageSize:          1000,
	}
}

// DefaultProtocolLimits preserves initial API name.
func DefaultProtocolLimits() *ProtocolLimits { return DefaultLimits() }
