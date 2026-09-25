package aws

// The analytics resources' main code paths: provisioned capacity needs its size.
func init() {
	attrs["aws_kinesis_stream"] = map[string]any{"shard_count": 2.0}
}
