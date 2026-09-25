# Scouters

| Type | Reads | Headroom |
| --- | --- | --- |
| `aws_lambda_function` | Requests, GB-seconds (x86_64 / arm64), logs | Concurrency (Little's law: peak rate × duration) against the account or reserved concurrency |
| `aws_api_gateway_rest_api` | Requests | Account throttle |
| `aws_apigatewayv2_api` | Requests in 512 KB steps (HTTP APIs) | Account throttle |
| `aws_cloudfront_distribution` | HTTPS requests, transfer out, by the price zone of the viewers (the region's zone unless set) | Requests per distribution |
| `aws_dynamodb_table` | On-demand request units or provisioned capacity (4 KB / 1 KB steps, consistency, transactions), storage | Table throughput or provisioned capacity |
| `aws_s3_bucket` | GET, PUT, storage (Standard), direct transfer out to the internet | Per-prefix request rate × prefixes |
| `aws_sqs_queue` | Requests in 64 KB chunks | FIFO send rate, by region in high throughput mode |
| `aws_sns_topic` | Publishes in 64 KB chunks | Publish rate |
| `aws_sfn_state_machine` | Transitions (standard), requests and GB-seconds (express) | StartExecution rate |
| `aws_rds_cluster` | Aurora Serverless v2 ACU-hours (provisioned instances are their own nodes), storage, I/O (Standard or I/O-Optimized), backup storage, Backtrack, snapshot export, the managed master password secret | Peak ACU against max capacity |
| `aws_rds_cluster_instance` | Provisioned Aurora instance-hours (Standard or I/O-Optimized), surplus CPU credits, Database Insights and Extended Support per vCPU | - |
| `aws_db_instance` | Instance-hours by engine, license and Single-AZ or Multi-AZ, storage (gp2, gp3, io1, io2, magnetic), IOPS and gp3 throughput above the baseline, magnetic I/O, backups beyond the free allowance, CPU credits, Database Insights, Extended Support, the managed master password secret | Storage IOPS against the volume |
| `aws_docdb_cluster` / `aws_neptune_cluster` | Storage (standard or I/O-Optimized), I/O, backups beyond the free allowance | Storage against the cluster limit |
| `aws_docdb_cluster_instance` / `aws_neptune_cluster_instance` | Instance-hours (standard or I/O-Optimized), serverless DCU or NCU-hours, CPU credits | - |
| `aws_docdb_cluster_snapshot` / `aws_neptune_cluster_snapshot` | Snapshot storage beyond the free allowance | - |
| `aws_elasticache_cluster` / `aws_elasticache_replication_group` | Node-hours by engine (Redis OSS, Valkey, Memcached) and node count, retained snapshots | Data set against node memory × shards |
| `aws_redshift_cluster` | Node-hours (doubled on Multi-AZ), managed storage, concurrency scaling, Spectrum, backups in three tiers | Stored data against the nodes' storage |
| `aws_dms_replication_instance` | Instance-hours (Single-AZ or Multi-AZ), storage above what the class includes | - |
| `aws_scheduler_schedule` | Invocations | - |
| `aws_cloudwatch_event_rule` | Nothing (scheduled rules are free) | - |
| `aws_ecs_task_definition` | Fargate vCPU and GB hours per run (1-minute minimum, x86_64 or arm64), ephemeral storage above 20 GB | vCPUs running against the Fargate quota, task launch rate, RunTask calls |
| `aws_ecs_service` | Fargate vCPU and GB hours of the tasks kept running (x86_64, arm64, Windows with its license), ephemeral storage above 20 GB, task size read through the task definition | vCPUs running against the On-Demand or Spot quota, tasks per service, requests against the tasks' capacity |
| `aws_eks_cluster` | Cluster-hours, extended support by Kubernetes version, Provisioned Control Plane tier, Auto Mode management per instance, control plane logs | - |
| `aws_eks_fargate_profile` | Fargate vCPU and GB hours of the pods it runs, ephemeral storage above 20 GB | vCPUs running against the Fargate quota |
| `aws_ecr_repository` | Image storage | Image pulls and layer downloads per second |
| `aws_kms_key` | Key-months with rotated versions, requests | Cryptographic requests per second (varies by region). References to a key are mentions: connect callers by hand |
| `aws_secretsmanager_secret` | Secret-months with replicas, API calls | GetSecretValue rate |
| `aws_ssm_parameter` | Advanced parameter-hours, API interactions (advanced or higher throughput) | GetParameter rate, standard or higher throughput |
| `aws_cognito_user_pool` | Monthly active users by feature plan (Lite tiered, Essentials, Plus) | Authentication and user creation rates |
| `aws_wafv2_web_acl` | Web ACL-months, rule-months, requests by inspection capacity | Requests per web ACL (regional) |
| `aws_cloudfront_function` | Invocations | - |
| `aws_vpc_endpoint` | Interface endpoint-hours per zone and data processed; Gateway endpoints are free | - |
| `aws_ec2_transit_gateway_vpc_attachment` | Attachment-hours and data processed, billed to the attachment owner | Bandwidth per zone |
| `aws_vpn_connection` | Connection-hours (standard or large tunnels), data sent out | Bandwidth per tunnel |
| `aws_nat_gateway` | Gateway-hours and data processed | Bandwidth |
| `aws_ec2_transit_gateway_peering_attachment` | Attachment-hours, data sent to the peer region at the inter-region price | Bandwidth per zone |
| `aws_dx_connection` | Port-hours by capacity and location (dedicated or hosted), data sent out to the location | Port bandwidth |
| `aws_dx_gateway_association` | Transit gateway attachment-hours and data processed; free with a virtual private gateway | Bandwidth per zone |
| `aws_networkfirewall_firewall` | Endpoint-hours per zone, data processed (in a VPC or on a transit gateway) | Bandwidth per zone |
| `aws_ec2_client_vpn_endpoint` / `aws_ec2_client_vpn_network_association` | Connection-hours / association-hours per subnet | Concurrent connections by subnets associated |
| `aws_ec2_traffic_mirror_session` | Mirrored interface-hours | - |
| `aws_flow_log` | Vended log delivery to CloudWatch Logs, S3 or Firehose, Parquet conversion | - |
| `aws_waf_web_acl` | WAF Classic: web ACL-months, rule-months, requests, at us-east-1 prices | - |
| `aws_lb` / `aws_alb` | Load balancer-hours and capacity units (LCU, NLCU, GLCU) from new and active connections, bytes and rule evaluations, reserved capacity, trust stores, public IPv4 addresses | - |
| `aws_elb` | Classic load balancer-hours, data processed, public IPv4 addresses | - |
| `aws_globalaccelerator_accelerator` / `aws_globalaccelerator_endpoint_group` | Fixed fee / data transfer premium in the dominant direction, by the endpoints' and the clients' location groups | - |
| `aws_route53_zone` / `aws_route53_record` | Hosted zone-months and record sets beyond 10,000 / queries by routing policy in two tiers (alias queries are free) | - |
| `aws_route53_health_check` | Health check-months for AWS or other endpoints, optional features | - |
| `aws_route53_resolver_endpoint` | Network interface-hours per IP address (DNS over HTTPS priced apart), queries in two tiers | Queries per second per IP address |
| `aws_api_gateway_stage` | The dedicated cache by size; requests pass on to the REST API | - |
| `aws_acm_certificate` / `aws_acmpca_certificate_authority` | Private certificate issues spread over the renewal period (public ones are free) / CA-months, certificates in tiers or short-lived, OCSP | IssueCertificate rate |
| `aws_cloudwatch_metric_alarm` | Alarm metric-months, standard or high resolution, anomaly detection | - |
| `aws_kinesis_stream` | On-demand stream-hours, data written and read, fan-out reads, retention; or provisioned shard-hours, PUT payload units, extended and long-term retention, fan-out consumer-shard-hours | Write MB/s and records against the shards, or the on-demand write ceiling |
| `aws_kinesis_firehose_delivery_stream` | Data ingested in volume tiers by source (Direct PUT, Kinesis, MSK) and for Iceberg, format conversion, dynamic partitioning, VPC delivery | Direct PUT records, requests and MiB per second (by region) |
| `aws_kinesisanalyticsv2_application` / `aws_kinesis_analytics_application` | KPU-hours, orchestration KPUs, running storage, durable backups (Flink, Studio or SQL); nothing while stopped | KPUs per application |
| `aws_kinesisanalyticsv2_application_snapshot` | Durable backup GB-months | - |
| `aws_msk_cluster` | Broker-hours by type (standard or Express), EBS storage, provisioned throughput, tiered storage; Express data in and storage | Data kept against the brokers' volumes |
| `aws_mq_broker` | Broker-hours by engine, deployment mode and type, EFS or EBS storage | - |
| `aws_opensearch_domain` / `aws_elasticsearch_domain` | Data, master and UltraWarm node-hours, EBS storage (gp2, gp3, io1, magnetic) with gp3 and io1 IOPS and throughput, managed storage | Data kept against the volumes |
| `aws_glue_job` | DPU-hours per run (1- or 10-minute minimum) or all month for streaming; standard, Flex, memory-optimized, Ray and Glue 6.0+ rates | Concurrent runs of the job, account DPUs |
| `aws_glue_crawler` | DPU-hours per crawl (10-minute minimum); the schedule becomes its load | Crawlers running in the account |
| `aws_glue_catalog_database` | Objects stored, requests | Tables per database |
| `aws_mwaa_environment` | Environment-hours by class, workers, schedulers and web servers beyond those included, metadata database | Concurrent tasks against the maximum workers' slots |
| `aws_pipes_pipe` | Requests in 64 KB chunks after the filter | - |
| `aws_bedrock_guardrail` | Text units per configured policy (content, topics, sensitive information, contextual grounding) | ApplyGuardrail and per-policy text units per second (varies by region) |
| `bedrock_model` | Input, output, cache read and cache write tokens (Claude 4.5 models), global or regional inference | Tokens per minute (output × burndown, cache reads excluded) and requests per minute |
| `aws_bedrockagentcore_agent_runtime` | Active vCPU-hours and peak-memory GB-hours per session (platform V1 or V2), logs | Concurrent sessions, session creation rate, data-plane calls, session length |
| `aws_bedrockagentcore_harness` | Session compute at Runtime prices, logs; managed memory asks for a Memory node | Runtime quotas: concurrent sessions, session creation, data-plane calls, session length |
| `aws_bedrockagentcore_policy_engine` | Authorization requests for the tool calls of its gateways, policy generation tokens | - |
| `aws_bedrockagentcore_registry` | Records stored, search calls | Search calls per second |
| `aws_bedrockagentcore_memory` | Short-term events, long-term records stored (built-in or custom strategy), retrievals | CreateEvent and retrieval rates, extraction tokens per minute |
| `aws_bedrockagentcore_gateway` | API invocations, search, tool indexing, VPC data processing | Tool calls and search calls per second |
| `aws_bedrockagentcore_code_interpreter` / `aws_bedrockagentcore_browser` | Session vCPU-hours and memory GB-hours | Concurrent sessions, session starts, invocations, session length |
| `aws_bedrockagentcore_workload_identity` | Credential requests, free through Runtime or Gateway | Token requests per second |
| `aws_bedrockagentcore_evaluator` / `aws_bedrockagentcore_online_evaluation_config` | Custom evaluations / sampled built-in evaluator tokens (on demand or batch) | Evaluation tokens and evaluations per minute |
| `agentcore_web_search` / `agentcore_knowledge_base` | Queries / retrievals and storage (external, placed by hand) | Query rate |
| `aws_codebuild_project` | Build minutes by environment and compute type, Lambda compute seconds, remote Docker server seconds | Concurrent builds per environment and compute type, build time against the timeout |
| `aws_cloudformation_stack` / `aws_cloudformation_stack_set` | Handler operations and handler time beyond 30 s for third-party and private registry types, per stack instance | - |
| `aws_cloudhsm_v2_hsm` | HSM-hours (hsm1.medium or hsm2m.medium) | RSA 2048 signatures per second per HSM (published guidance) |
| `aws_directory_service_directory` | Directory-hours (Simple AD, AD Connector), domain controller-hours and sharing (Microsoft AD) | - |
| `aws_grafana_workspace` | Editor, viewer and Enterprise plugins licenses | - |
| `aws_kms_external_key` | Key-months, requests | Cryptographic requests per second (varies by region) |
| `aws_ssm_activation` | Session Manager sessions and Run Command invocations on hybrid nodes | - |
| `aws_instance` / `aws_spot_instance_request` | Instance-hours by OS (Linux, Windows, RHEL, SUSE) and tenancy, spot as a share of on-demand, root and first data volume, EBS-optimized surcharge, detailed monitoring, unlimited CPU credits, public IPv4 | Peak against what one instance serves |
| `aws_autoscaling_group` | Instance-hours split on-demand/spot by the mixed instances policy, root volumes, monitoring, credits, public IPv4 | Peak against max_size instances |
| `aws_eks_node_group` | Node-hours (on-demand or spot, OS from ami_type), node disks | Peak against max_size nodes |
| `aws_elastic_beanstalk_environment` | From its settings: instance-hours (on-demand/spot), root volumes, the Application, Network or Classic load balancer, streamed logs | Peak against MaxSize instances |
| `aws_ec2_host` / `aws_lightsail_instance` / `aws_eip` | Dedicated Host-hours by family / bundle-hours / public IPv4 address-hours | - / peak against the instance / - |
| `aws_ebs_volume` | GB-months by type, provisioned IOPS (io2 in three tiers) and gp3 throughput, magnetic I/O | Peak I/O against the volume's IOPS |
| `aws_ebs_snapshot` / `aws_ebs_snapshot_copy` | Standard or archive storage, archive restores, fast snapshot restore, EBS direct API calls | Direct API rates per snapshot and account |
| `entry` | Nothing; checks that load is set | - |

Zone crossings are read by the VPC, not by a node. Set `kb` on an edge between
two nodes inside it (a function and its database) and `zones` on the VPC: with
the nodes spread evenly, two in three calls over three zones cross. Leave `kb`
off edges to regional services such as S3 and DynamoDB, which do not cross
zones. A `kb` on an edge that leaves the group is reported, not read. See
[`examples/private-network`](../examples/private-network).

Network resources sit on the path. Terraform cannot tell which calls go
through a transit gateway, VPN, NAT gateway or endpoint, so draw the edge
through the node by hand: load that reaches it carries `kbPerUnit` each, turns
into GB and bandwidth, and flows on to the next node, which then counts the
hop in the path's latency and availability. `gbPerMonth` adds traffic that is
not drawn as load. A node that load reaches without `kbPerUnit` is an error,
not zero.

## Azure

| Type | Reads | Headroom |
| --- | --- | --- |
| `azurerm_linux_function_app` / `azurerm_windows_function_app` | Consumption executions and GB-seconds (other plans are priced on the plan) | Instances and timeout |
| `azurerm_function_app_flex_consumption` | On-demand executions and GB-seconds, always-ready baseline | Instances per function group |
| `azurerm_service_plan` | Instance-hours by SKU and OS, times workers | - |
| `azurerm_function_app` | Legacy function app: Consumption executions and GB-seconds (other plans are priced on the plan) | Instances and timeout |
| `azurerm_app_service_plan` | Legacy plan: instance-hours by SKU (Shared to Isolated v4) and OS, or Elastic Premium vCPU and memory hours | - |
| `azurerm_app_service_environment` | Stamp fee and one instance of the pricing tier | - |
| `azurerm_app_service_certificate_order` / `azurerm_app_service_certificate_binding` / `azurerm_app_service_custom_hostname_binding` | Certificate per year / IP-based SSL bindings per month | - |
| `azurerm_static_web_app` / `azurerm_static_site` | Standard plan fee and bandwidth above 100 GB | Free plan bandwidth |
| `azurerm_container_registry` | Registry units by SKU, geo-replicas, storage above the included amount, build vCPU time | Read and write request rates, storage |
| `azurerm_logic_app_standard` | Workflow Standard vCPU and memory hours, connector calls | - |
| `azurerm_logic_app_integration_account` / `azurerm_integration_service_environment` | Account fee by SKU / base and scale unit-hours | - |
| `azurerm_container_app` | Consumption vCPU- and GiB-seconds, active and idle, requests | Replicas |
| `azurerm_storage_account` | Hot tier storage, write and read operations by redundancy, transfer out to the internet | Account request rate (varies by region) |
| `azurerm_cosmosdb_account` | Serverless request units (1.25× with zones); RU/s-hours and storage of databases and containers not declared | Provisioned RU/s |
| `azurerm_cosmosdb_sql_database` / `_sql_container`, `_mongo_database` / `_mongo_collection`, `_cassandra_keyspace` / `_cassandra_table`, `_gremlin_database` / `_gremlin_graph`, `azurerm_cosmosdb_table` | Manual or autoscale RU/s-hours (multi-region writes, zone redundancy), storage and analytical store per region, periodic or continuous backup, restores | RU/s against their throughput |
| `azurerm_redis_cache` | Node-hours by tier and size, shards × (1 + replicas) in Premium | Memory and client connections by size, per shard |
| `azurerm_managed_redis` | Instance-hours by SKU, doubled with high availability | Memory and client connections by SKU |
| `azurerm_search_service` | Search units (replicas × partitions) by tier, semantic ranker queries, image extraction tiers | Index size against partition storage |
| `azurerm_postgresql_flexible_server` | Compute by SKU (doubled with high availability), storage | Connections by SKU |
| `azurerm_mssql_database` / `azurerm_sql_database` | vCore hours (serverless: vCore-hours used), SQL license, zone redundancy, Hyperscale replicas, storage; or DTU objective per day and extra storage; backups. In a pool, backups only | Concurrent workers |
| `azurerm_mssql_elasticpool` / `azurerm_sql_elasticpool` | eDTUs per day and extra storage, or vCore hours, SQL license, zone redundancy, storage | - |
| `azurerm_mssql_managed_instance` / `azurerm_sql_managed_instance` | vCore hours, SQL license, zone redundancy, storage above 32 GB, backups | Concurrent workers |
| `azurerm_mysql_flexible_server` | Compute by SKU (doubled with high availability), storage, IOPS above 360, backups | Connections by SKU |
| `azurerm_mysql_server` / `azurerm_postgresql_server` / `azurerm_mariadb_server` | Single servers (retired): vCore hours by tier, storage, backups | Connections by SKU |
| `azurerm_api_management` | Consumption calls, or unit-hours by tier with included calls | Requests per unit (published guidance) |
| `azurerm_cdn_frontdoor_profile` | Base fee, requests and transfer out by the viewers' zone | - |
| `azurerm_frontdoor` | Classic: routing rule-hours, frontend hosts beyond 100, transfer in, tiered transfer out by the viewers' zone | Requests, bandwidth, routing rules, frontend hosts |
| `azurerm_frontdoor_firewall_policy` | Classic WAF: policy, custom rules and managed rule sets, the requests they evaluate | Custom rules |
| `azurerm_cdn_endpoint` | Classic Standard from Microsoft: tiered transfer out by the viewers' zone, rules beyond 5 and rules engine requests | Requests, bandwidth, rules |
| `azurerm_dns_zone` | Public zone-months, record sets beyond 10,000 | Record sets |
| `azurerm_dns_a_record` ... `azurerm_dns_txt_record` (A, AAAA, CAA, CNAME, MX, NS, PTR, SRV, TXT) | Queries, first billion and beyond | Records per set |
| `azurerm_traffic_manager_profile` | DNS queries, Traffic View data points | - |
| `azurerm_traffic_manager_azure_endpoint` / `azurerm_traffic_manager_external_endpoint` | Health checks, fast interval and HTTPS add-ons | - |
| `azurerm_traffic_manager_nested_endpoint` | Nested endpoint-months | - |
| `azurerm_servicebus_namespace` | Operations (Basic, Standard with its base fee), or Premium messaging units | Operations per second |
| `azurerm_eventgrid_topic` | Operations | Events per second |
| `azurerm_key_vault` | Secret operations | Requests per vault |
| `azurerm_log_analytics_workspace` | Ingestion and retention beyond 31 days | - |
| `azurerm_key_vault_key` | Operations (RSA 2048 or advanced), HSM key-months per version, automatic rotations | Vault rate for the key type and protection |
| `azurerm_key_vault_certificate` | Renewals read from the policy, operations | Requests per vault |
| `azurerm_key_vault_managed_hardware_security_module` | HSM pool hours (keys and operations included) | RSA 2048 unwrap throughput |
| `azurerm_cognitive_deployment` | Azure OpenAI input, cached and output tokens by model and deployment type | Tokens and requests per minute from capacity or quota |
| `azurerm_linux_virtual_machine` / `azurerm_windows_virtual_machine` / `azurerm_virtual_machine` | Instance hours by size (Linux, Windows, or the base rate with Hybrid Benefit), OS disk tier and operations, inline data disks (legacy), Ultra Disk reservation | - |
| `azurerm_linux_virtual_machine_scale_set` / `azurerm_windows_virtual_machine_scale_set` / `azurerm_virtual_machine_scale_set` | Instance hours by size times instances, their OS and data disks | - |
| `azurerm_managed_disk` | Standard HDD, Standard SSD and Premium SSD by tier with operations; Ultra and Premium SSD v2 capacity, IOPS and throughput | IOPS against the tier or the provisioned IOPS |
| `azurerm_snapshot` / `azurerm_image` | Data held, at the snapshot price | - |
| `azurerm_backup_protected_vm` / `azurerm_recovery_services_vault` | Protected instances by data size / backup storage by the vault's redundancy | - |
| `azurerm_kubernetes_cluster` / `azurerm_kubernetes_cluster_node_pool` | Standard tier uptime SLA or Premium long-term support, node hours by size and OS, managed OS disks, load balancer data, the HTTP routing DNS zone | - |
| `azurerm_lb` | Standard and cross-region: the fee covering the first five rules, data processed; gateway: gateway-, chain-hours and data processed (Basic is free) | - |
| `azurerm_lb_rule` / `azurerm_lb_outbound_rule` | Rule-hours for each rule beyond a load balancer's first five | - |
| `azurerm_application_gateway` | v2: fixed hours and capacity units, reserved or used (throughput, compute units); v1: instance-hours and data processed beyond the free allowance | Instances at peak against the autoscale maximum or manual count |
| `azurerm_public_ip` / `azurerm_public_ip_prefix` | Address-hours by SKU, tier and allocation / address-hours for every address in the prefix | - |
| `azurerm_nat_gateway` | Gateway-hours and data processed (Standard and StandardV2) | Bandwidth |
| `azurerm_bastion_host` | Host-hours by SKU, instances beyond two, data out | Concurrent RDP or SSH sessions per instance |
| `azurerm_firewall` | Deployment-hours and data processed by tier, in a VNet or a secured hub | Throughput by tier |
| `azurerm_firewall_policy` | The fee per region once two or more firewalls use it, policy analytics; rule collection groups fold in | - |
| `azurerm_private_endpoint` | Endpoint-hours and data processed | - |
| `azurerm_virtual_network_peering` | Data sent and received, within a region or global | - |
| `azurerm_sentinel_data_connector_*` (8 connectors) | Sentinel pay-as-you-go ingestion per GB, including the Log Analytics charge; free data sources (alerts, most Office 365 audit logs) read nothing | - |
| `azurerm_security_center_subscription_pricing` | The Defender for Cloud plan's unit: servers, instances, accounts (with transaction overage and malware scanning), vaults, subscriptions, vCores, images, RU/s, queries or tokens | - |
| `azurerm_active_directory_domain_service` / `_replica_set` | Hours by SKU, per replica set | Recommended authentications per hour |
| `azurerm_application_insights` | Telemetry after sampling and the daily cap, and retention beyond 90 days, at the workspace's prices when workspace-based or classic prices | Events per second |
| `azurerm_monitor_action_group` | Emails, push, ITSM events, webhooks (plain and secure), SMS and voice calls by country code, per time fired | Emails, SMS and calls per address or number |
| `azurerm_monitor_metric_alert` | Time series monitored (resources in scope times criteria or dimension values), dynamic thresholds | - |
| `azurerm_monitor_scheduled_query_rules_alert` / `azurerm_monitor_scheduled_query_rules_alert_v2` | The rule by evaluation frequency / and time series beyond the first | - |
| `azurerm_monitor_data_collection_rule` | Custom metric samples; logs flow on to the workspace | - |
| `azurerm_monitor_diagnostic_setting` | Platform logs sent to storage, an event hub or a partner | - |
| `azurerm_log_analytics_solution` | Microsoft Sentinel analysis per GB; other solutions have no price of their own | - |
| `azurerm_automation_account` | Job minutes and non-Azure configuration nodes | Job submissions and concurrent jobs |
| `azurerm_automation_job_schedule` / `azurerm_automation_watcher` | Job minutes per run / watcher hours | - |
| `azurerm_automation_dsc_configuration` / `azurerm_automation_dsc_nodeconfiguration` | Non-Azure configuration nodes | - |
| `azurerm_eventgrid_system_topic` | Operations, as a custom topic | - |
| `azurerm_eventhub_namespace` | Throughput-unit hours, ingress events and Capture (Basic, Standard), processing-unit hours (Premium), capacity-unit hours (Dedicated), retention beyond the included | Ingress per throughput unit, up to the auto-inflate ceiling |
| `azurerm_iothub` | Units by tier (F1 is free) | Daily messages and device-to-cloud sends per unit |
| `azurerm_iothub_dps` | Operations | Registrations per minute per unit |
| `azurerm_notification_hub_namespace` | Base fee and pushes beyond the included 10 million, by tier | Active devices, the free tier's pushes |
| `azurerm_signalr_service` | Unit-days and messages beyond those included, per 2 KB | Concurrent connections per unit |
| `azurerm_app_configuration` | Store- and replica-days, requests beyond the daily allowance | Requests per hour or day by tier, read rate |
| `azurerm_storage_queue` | Data stored, Class 1 and 2 operations by redundancy, geo-replication transfer | Messages per second per queue |
| `azurerm_storage_table` | Data stored, write, batch, read, scan, delete and list operations by redundancy and encryption | Entities per second per partition |
| `azurerm_storage_share` | Data, snapshots and metadata stored, operations and cool retrieval by tier and redundancy; premium provisioned size | IOPS per share |
| `azurerm_storage_management_policy` | Tier changes billed as the destination tier's writes | - |

Role assignments become edges: an `azurerm_role_assignment` connects the
resource whose managed identity holds the role (system- or user-assigned) to
the resource it is scoped to, with the kinds the role grants. Each resource's
`iam` lists Azure role names per kind, where AWS resources list IAM actions.

## Google Cloud

| Type | Reads | Headroom |
| --- | --- | --- |
| `google_cloud_run_v2_service` | Request-based vCPU- and GiB-seconds from the container limits and concurrency, requests | Instances against max instances |
| `google_cloud_run_service` | The same from the v1 template, idle minimum instances, or instance-based vCPU- and GiB-seconds with CPU always allocated | Instances against max instances |
| `google_cloud_run_v2_job` | Instance-based vCPU- and GiB-seconds per task, a minute at least | Job runs per minute, running executions |
| `google_cloudfunctions2_function` | Cloud Run prices for its CPU and memory, invocations | Instances against max instances |
| `google_cloudfunctions_function` | 1st gen GB- and GHz-seconds in 100 ms steps, invocations, idle minimum instances, outbound data (its event trigger calls it) | Instances against max instances |
| `google_cloud_scheduler_job` | Job-months | - |
| `google_cloud_tasks_queue` | Operations in 32 KB chunks | Dispatch rate against max dispatches |
| `google_storage_bucket` | Standard storage in a region, Class A and B operations, transfer out to the internet | Initial read and write rates per bucket |
| `google_firestore_database` | Document reads, writes and deletes, stored data (Native mode, Standard edition) | - |
| `google_sql_database_instance` | vCPU and memory hours or a shared-core instance, SSD, with high-availability prices | PostgreSQL connections by memory |
| `google_pubsub_topic` | Publish throughput, and delivery to subscriptions not declared | Publish throughput per region (varies by region) |
| `google_pubsub_subscription` | Delivery or BigQuery and Cloud Storage export throughput, retained acknowledged messages, snapshots, backlog (the topic calls it) | Pull, push or export throughput per region |
| `google_secret_manager_secret` | Versions not declared, access operations | Access requests per minute |
| `google_secret_manager_secret_version` | One version per replica location, access operations | Access requests per minute |
| `google_kms_crypto_key` | Key versions by protection level and algorithm (rotation adds them), cryptographic operations | Cryptographic requests per minute |
| `google_bigquery_dataset` | On-demand queries per TiB scanned, in a region or the US or EU multi-region | - |
| `google_bigquery_table` | Active and long-term storage, streaming inserts or Storage Write API, queries, Storage Read API | Streaming throughput per project |
| `google_redis_instance` | GiB-hours by tier and capacity tier, per node with read replicas | Network throughput by capacity tier |
| `google_redis_cluster` | Node-hours by node type for shards and replicas, AOF persistence, backups | Client connections per node |
| `google_artifact_registry_repository` | Storage, transfer to other locations by continent and to the internet | Requests and write requests per minute |
| `google_container_registry` | The multi-region bucket behind it: Standard storage, Class A and B operations, transfer out | Initial read and write rates per bucket |
| `google_api_gateway_gateway` | Calls | Quota units per second |
| `google_compute_forwarding_rule` / `google_compute_global_forwarding_rule` | Rule-hours (the first five of a project share one minimum), data processed by passthrough rules, proxy instances for INTERNAL_MANAGED rules; Private Service Connect endpoint-hours and data processed | - |
| `google_compute_target_http_proxy` / `_https_proxy` / `google_compute_region_target_http_proxy` / `_https_proxy` | Data processed, at the external or internal Application Load Balancer price | - |
| `google_compute_target_tcp_proxy` / `google_compute_target_ssl_proxy` | Data processed by the proxy Network Load Balancer | - |
| `google_compute_target_grpc_proxy` | Cloud Service Mesh client-hours | - |
| `google_compute_router_nat` | Public NAT uptime by VM instances (capped from 33), NAT IP-hours, data processed; Private NAT uptime and data | Source ports on manual NAT addresses |
| `google_compute_vpn_tunnel` | Tunnel-hours | - |
| `google_compute_vpn_gateway` / `google_compute_ha_vpn_gateway` / `google_compute_external_vpn_gateway` | IPsec traffic sent: internet transfer out to a device outside Google Cloud (read on the External VPN gateway for HA VPN), between-zones to a gateway in the same region | - |
| `google_service_networking_connection` | Data crossing zones to a Google-managed service network | - |
| `google_dns_managed_zone` / `google_dns_record_set` | Zone-months by the account's zone count / queries, with routing policies priced higher | - |
| `gemini_model` | Gemini 2.5 input, cached and output tokens (placed by hand) | Tokens per minute |
| `google_logging_{project,folder,organization,billing_account}_sink` | Logging storage for a log bucket written out as its destination; routing elsewhere is free and billed by the destination | Log write rate per project and region |
| `google_logging_{project,folder,organization,billing_account}_bucket_config` | Logging storage of what sinks route in, retention beyond 30 days | - |
| `google_monitoring_metric_descriptor` | Metric volume in bytes ingested by tier, read API time series | One point every 5 seconds per time series, active time series |
| `google_compute_instance` | Machine type hours (predefined, or custom vCPUs, memory and extended memory), on demand or Spot, with sustained use discounts; boot disk capacity, IOPS and throughput; local SSDs; GPUs on N1; an ephemeral external IP | Requests against what the VM serves |
| `google_compute_instance_group_manager`, `google_compute_region_instance_group_manager` | The same per instance, read from the instance template (its first disk as the boot disk, the other disks as data disks, SCRATCH disks as local SSDs), times the target size | Requests against the instances |
| `google_compute_per_instance_config`, `google_compute_region_per_instance_config` | One more instance of the group, read through the group from its template | - |
| `google_compute_disk` | Persistent Disk and Hyperdisk capacity, provisioned IOPS and throughput (type defaults for the size) | IOPS against the disk's limit |
| `google_compute_image`, `google_compute_machine_image`, `google_compute_snapshot` | Image, machine image and standard or archive snapshot storage (regional or multi-regional) | - |
| `google_compute_address`, `google_compute_global_address` | External IPv4 hours by what uses the address (VM, Spot VM, unused); internal and forwarding-rule addresses are free | - |
| `google_container_cluster` | Management fee, Autopilot Pod vCPU, memory and ephemeral storage, the default or first inline node pool's nodes | - |
| `google_container_node_pool` | Nodes per zone times zones (its own, else the cluster's): instance hours, boot disks, local SSDs, GPUs, public nodes' IPs | Requests against the nodes |

Every Google Cloud price is checked against the Billing Catalog, 13,217
cells in all. Three have no SKU to check against and are read from the
pricing pages, with notes saying so: Cloud DNS routing-policy queries, the
Cloud NAT gateway cap and free log routing. A machine type is priced as the
SKUs Compute Engine bills it by, its vCPUs, memory, GPUs (A2 and A3 included),
bundled Local or Titanium SSD and the M2 premium, taken from the
machineTypes API with the regions each type is offered in.

AWS and Azure prices are checked the same way, entry by entry against the
public price lists: 179,124 AWS cells and 76,477 Azure cells, including the
volume tiers each product bills by. Six AWS prices have no usage type in the
Price List (AgentCore policy and registry, Systems Manager hybrid activations)
and are read from the pricing pages, with notes saying so. A cell left empty
is a product not sold in that region.

## Cloudflare

| Type | Reads | Headroom |
| --- | --- | --- |
| `cloudflare_workers_script` | Requests and CPU milliseconds (routes, custom domains and cron triggers fold in) | CPU time per request |
| `cloudflare_workers_paid_plan` | The Workers Paid base fee, one per account (placed by hand) | - |
| `cloudflare_durable_object` | Requests, duration of active objects, SQLite rows read and written, storage (placed by hand) | Requests per second per object |
| `workers_ai_model` | Input, cached and output tokens by model (placed by hand) | Requests per minute |
| `cloudflare_zone` | The plan's monthly fee (DNS records and settings fold in) | - |
| `cloudflare_r2_bucket` | Storage, Class A and B operations, Infrequent Access retrieval; egress is free | Writes per second to one key |
| `cloudflare_d1_database` | Rows read and written, storage | Database size; queries per second from one query at a time |
| `cloudflare_workers_kv_namespace` | Reads, writes (deletes and lists cost the same), storage | Writes per second to one key |
| `cloudflare_queue` | Operations in 64 KB chunks | Messages per second per queue, message size |

Cloudflare publishes no price API, so `sync` cannot check these prices. They
are read from the pricing pages by hand and marked verified with the date they
were read. Each price is the same everywhere (`*`), so a Cloudflare node prices
in the region of any declaration, and a declaration of Cloudflare alone needs
no region. The amounts the Workers Paid plan includes (Workers requests and
CPU time, Workers KV, Durable Objects, D1, Queues) and R2's free tier are free
units counted over the whole account: every node of a declaration shares
them, and the plan's $5 fee is paid once. The Workers AI neurons (10,000 a
day) are not subtracted.

`archgopher catalog` prints every scouter with its fields as JSON.

## Adding a resource

Every resource is a directory in [`catalog/aws`](../catalog/aws),
[`catalog/azure`](../catalog/azure), [`catalog/gcp`](../catalog/gcp) or
[`catalog/cloudflare`](../catalog/cloudflare), named after its type. It holds
everything about that resource and nothing else; adding one needs no Go code.
Its `icon` names a picture in `web/src/ui/icons/<provider>/`; a test fails when
a resource has none or a picture is unused.

```text
catalog/aws/aws_sqs_queue/
  resource.yaml      its icon, what it accepts, how load becomes readings, Terraform rules, IAM actions
  books/prices.json  its prices, with the Price List filters that verify them
  books/quotas.json  its quotas
  books/slas.json    its SLA
  cases.yaml         worked examples: these values and this load read these costs
```

A definition composes facets (the L2 readings in [`facet`](../facet)). Numbers
are expressions; text can embed `{expressions}`. Expressions are checked when
the catalog loads, so a typo or a type mismatch never reaches a user.

```yaml
type: aws_sqs_queue
kinds: [send]
attributes:
  - { key: fifo_queue, label: FIFO, type: boolean, default: false }
assumptions:
  - { key: messageKb, label: Message size, type: number, unit: KB, default: 1 }
readings:
  - requests:
      name: Requests
      price: 'aws.sqs.{fifo_queue ? "fifo" : "standard"}.requests'
      count: total.monthly * 3
      chunkKb: 64
      sizeKb: messageKb
  - rate: { when: fifo_queue, name: FIFO send rate, unit: messages/second, quota: aws.sqs.fifo.tps, peak: total.peak }
iam:
  send: ["sqs:SendMessage"]
```

| Reading | What it records |
| --- | --- |
| `requests` | Requests, optionally billed in size chunks |
| `compute` | GB-seconds, optionally in memory steps |
| `storage` / `capacity` | GB-months / units provisioned all month |
| `rate` | Peak rate against a quota, optionally scaled |
| `concurrency` | Little's law: peak × duration against a quota or a set capacity |
| `logs` / `tokens` | Log ingestion and retention / model tokens with cache and burndown |
| `session` | Session compute: active vCPU-hours, peak-memory GB-hours, concurrent sessions |
| `cost` / `limit` | Any quantity × price / any demand against a quota or capacity |
| `fail` | A problem with the declaration; stops the node unless `continue: true` |

Expressions see every attribute and assumption by key (optional ones are nil
when unset), `total.monthly` and `total.peak`, `demand.<kind>.monthly` and
`.peak`, `region`, earlier `let` values, and `ceilDiv(a, b)`. `includes: [logs]` adds a
facet's own assumption fields. A directory without `resource.yaml` holds rows
several resources share, such as log prices.

An attribute's `path` says where Terraform keeps it:

| Path | Reads |
| --- | --- |
| `sku.name`, `boot_disk.initialize_params.size` | The value in the first block, through nested blocks |
| `setting[name=InstanceType].value` | The first block whose `name` is `InstanceType` (Beanstalk options) |
| `task_definition->cpu` | `cpu` on the resource the attribute references (a service's task size); `->` may repeat up to four hops |
| `criteria.dimension.values.#` | How many values are written, across every block |
| `version.instance_template->disk.*.disk_size_gb` | A `list`: the value in every block, `""` where a block leaves it unset, so several lists line up |
| a number pointing at repeated blocks | How many are written (a firewall's `subnet_mapping`) |
| a boolean pointing at a block, a reference or an id | Whether it is written, even when the id is known only after apply |

Prices that differ by one attribute only (an instance type, a database
class) are a table: one entry with `rows` instead of `values`, one number per
region, `null` where the row is not offered. Row `t3.micro` of
`aws.ec2.linux` is priced as `aws.ec2.linux.t3.micro`, so a reading names it
with `'aws.ec2.linux.{instance_type}'`. `{row}` in the sync filters stands for
the row key, so one spec verifies every row. When the provider lists no price for the row
itself, `compose` builds it from prices it does list: a Compute Engine
machine type is so many vCPU-hours, GiB-hours of memory, GPU-hours and SSD.
`{part}` in the filters stands for each part's name (or its `match`
pattern); `with` lays keys over the spec for one part (a license sold
`global`) and `withIn` for one region, where two prices share a name and
only a SKU id tells them apart. `in` lists the regions the row is offered
in, and `composeOf` reuses another table's parts (the Spot prices of the
same machines). `{region}` stands for the region
being verified, for offers that list a price from both ends (data sent between
regions: `"fromRegionCode": "{region}"`).

```json
"aws.ec2.linux": {
  "unit": "instance-hour", "source": "https://aws.amazon.com/ec2/pricing/on-demand/",
  "sync": {"service": "AmazonEC2", "filters": {"instanceType": "{row}", "operatingSystem": "Linux", "tenancy": "Shared", "preInstalledSw": "NA", "capacitystatus": "Used"}},
  "rows": {"t3.micro": {"us-east-1": 0.0104, "ap-northeast-1": 0.0136}},
  "verified": true, "checkedAt": "2026-09-25"
}
```

Most volume tiers and free allowances belong to the account, not to one
resource: two buckets share the storage tiers, and the Lambda free requests
are given once. An entry says so with its pricing rules, and the engine bills
such a price once, over every line of the declaration that reads it, then
shares the cost out by quantity. The declaration is one account.

| Field | Means |
| --- | --- |
| `pool` | `account` (the whole declaration) or `region` (each region of it): the lines that read the price share its tiers and free units |
| `tiered` | The table's rows are volume tiers: a row key is where the tier starts, in the entry's unit, counted over the pool's whole usage, and `"tier": "{row}"` in the sync spec verifies each one (a provider that counts in larger units, `listPer`, has its starts converted) |
| `free` | Units that cost nothing each month, given once per pool (a plan's included amount, an always-free allowance) |
| `freeGroup` | Free units several prices share (the Lambda free GB-seconds cover both architectures, the CloudFront free terabyte every price zone): each pool of the group gets a share by its quantity |
| `combine: max` | A fee the pool pays once however many lines need it: billed for the largest quantity, not the sum (a regional fee while any dedicated instance runs) |

A pooled line pays the pool's average price, and the result lists each pool
with its bands and the lines that share it. `billing: {free: false}` in the
declaration bills free units like any other, and a free grant the provider
lists as a zero-priced first tier at the first paid tier's price, for an
account whose organization uses them up elsewhere. A node read alone (its cases, `gaps`)
is billed as the only user of its pools.

```json
"aws.cloudfront.jp.transfer_out": {
  "unit": "GB", "pool": "account", "tiered": true, "source": "https://aws.amazon.com/cloudfront/pricing/",
  "sync": {"service": "AmazonCloudFront", "offerRegion": "aws-other", "tier": "{row}", "filters": {"usagetype": "JP-DataTransfer-Out-Bytes"}},
  "rows": {"0": {"*": 0.114}, "10240": {"*": 0.089}, "51200": {"*": 0.086}},
  "verified": true, "checkedAt": "2026-09-25"
}
```

`go test ./provider/aws` loads the catalog, runs every resource's cases and
checks that every row belongs to one directory.
