# Reference books

Prices, quotas and SLAs live next to each resource in `catalog/<provider>/<type>/books`, one row per ID with
a value per region, a unit, a source URL and a `verified` flag.

- **Units are checked.** A reading that counts `GB` against a price per
  `GB-month` is an error, not a wrong number.
- **Unverified values are listed.** Any value nobody has checked, or that is
  unknown, appears at the end of every report.
- **Prices sync from the public price lists.** `archgopher sync` reads the
  AWS Price List bulk files and the Azure Retail Prices API (neither needs
  credentials) and the Google Cloud Billing Catalog, and marks each price
  verified. The Billing Catalog needs credentials: set `ARCHGOPHER_GCP_TOKEN`,
  `ARCHGOPHER_GCP_ACCOUNT` (a gcloud account to take a token from) or
  `ARCHGOPHER_GCP_API_KEY`. Without them, Google Cloud rows are skipped, not
  failed, and keep their values.
  A weekly workflow opens a pull request when a price changes.
  `archgopher explore <service> <region> [attr=regex...]` helps you write the
  filters for a new price.

```sh
archgopher sync --check      # exit 1 if the book is out of date
archgopher sync --add-regions eu-west-2   # add a region to every price
archgopher explore AWSLambda ap-northeast-1 'usagetype=.*GB-Second.*'
archgopher explore azure Functions japaneast 'meterName=Standard.*'
ARCHGOPHER_GCP_ACCOUNT=you@example.com archgopher explore gcp 152E-C115-5142 asia-northeast1
```

**Regions.** Ten AWS regions are covered: us-east-1, us-east-2, us-west-2,
eu-west-1, eu-central-1, ap-northeast-1, ap-northeast-2, ap-southeast-1,
ap-southeast-2 and ap-south-1. Ten Azure regions match them: eastus, eastus2,
westus2, northeurope, germanywestcentral, japaneast, koreacentral,
southeastasia, australiaeast and centralindia. Ten Google Cloud regions match
them too: us-east4, us-east5, us-west1, europe-west1, europe-west3,
asia-northeast1, asia-northeast3, asia-southeast1, australia-southeast1 and
asia-south1. A row that is the same everywhere is `*`; a
quota that differs names its regions over a `*` default ("2,500 elsewhere").
`sync --add-regions` reads every price from the Price List for the new region
and lists anything left to fill by hand. Where the Price List has no price, the
row is recorded as not offered, and a node that needs it fails with "not
offered in <region>" instead of costing nothing. A test keeps every region
complete.

Quotas are the published defaults. Some are account-specific in practice
(Lambda concurrency on new accounts, Bedrock tokens per minute), and their
notes say so. AgentCore publishes no SLA, so its availability stays unknown. A value AWS does not publish stays unknown: the report shows
the demand and says the capacity is unknown.
