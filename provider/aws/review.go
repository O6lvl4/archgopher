package aws

import "github.com/O6lvl4/archgopher/review"

// ReviewRules are the AWS practices `review` reads the graph against.
func ReviewRules() []review.Rule {
	return []review.Rule{
		{
			ID:     "aws.lambda.one-queue-per-consumer",
			Pillar: "Serverless Lens / operational excellence",
			Title:  "one Lambda function consumes several SQS queues",
			Hint: "One consumer, one queue: tell kinds of work apart by a message attribute. " +
				"Separate queues earn their place when the consumers differ (timeout, permissions, maximum concurrency, retention), " +
				"and then each queue usually has its own function. If priority is the reason, write down why one queue cannot do.",
			Source: "https://docs.aws.amazon.com/wellarchitected/latest/serverless-applications-lens/welcome.html",
			FanIn:  &review.FanIn{Node: "aws_lambda_function", From: "aws_sqs_queue", Max: 1},
		},
	}
}
