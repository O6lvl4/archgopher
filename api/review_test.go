package api

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/cloud"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// TestReviewReadsTerraform runs every AWS rule over a configuration where
// each "bad" resource departs from one rule and each "good" one meets it.
func TestReviewReadsTerraform(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "review"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, cloud.TerraformRules(), "review")
	found, err := Review(spec, ev)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range found {
		got = append(got, f.Rule+" "+f.Node)
	}
	sort.Strings(got)
	want := []string{
		"aws.dynamodb.point-in-time-recovery aws_dynamodb_table.bad",
		"aws.ecr.lifecycle aws_ecr_repository.bad",
		"aws.ecr.scan-on-push aws_ecr_repository.bad",
		"aws.ecr.tag-immutability aws_ecr_repository.bad",
		"aws.iam.role-per-function aws_iam_role.shared",
		"aws.kms.rotation aws_kms_key.bad",
		"aws.lambda.error-alarm aws_lambda_function.lonely",
		"aws.lambda.sqs-maximum-concurrency aws_lambda_event_source_mapping.bad",
		"aws.lambda.sqs-partial-batch aws_lambda_event_source_mapping.bad",
		"aws.logs.retention aws_cloudwatch_log_group.bad",
		"aws.rds.backup-retention aws_rds_cluster.bad",
		"aws.rds.deletion-protection aws_rds_cluster.bad",
		"aws.rds.storage-encrypted aws_rds_cluster.bad",
		"aws.s3.public-access-block aws_s3_bucket.bad",
		"aws.sfn.failure-alarm aws_sfn_state_machine.bad",
		"aws.sfn.logging aws_sfn_state_machine.bad",
		"aws.sqs.dead-letter-alarm aws_sqs_queue.unwatched_dead",
		"aws.sqs.dead-letter-queue aws_sqs_queue.bad",
		"aws.sqs.visibility-timeout aws_lambda_event_source_mapping.bad",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestReviewDeclarationOnly reads a declaration file with no Terraform: only
// the graph rules apply.
func TestReviewDeclarationOnly(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "review"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, cloud.TerraformRules(), "review")
	found, err := Review(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("graph rules found %v in a configuration of one queue per consumer", found)
	}
}
