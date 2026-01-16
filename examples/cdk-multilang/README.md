# DynamORM CDK Multi-language Demo

Deploys **one DynamoDB table** and **three Lambdas** (Go, Node.js 24, Python 3.14) that read/write the same item
shape. This is the deployable “proof” that the multi-language DynamORM stack can share a single table without drift.

## Commands

From the repo root:

- Install deps: `npm --prefix examples/cdk-multilang ci`
- Synthesize: `npm --prefix examples/cdk-multilang run synth`
- Deploy: `npm --prefix examples/cdk-multilang run deploy`

After deploy, the stack outputs three Function URLs. Use them to `GET`/`PUT` items:

- `GET ?pk=...&sk=...`
- `PUT` with JSON body: `{"pk":"...","sk":"...","value":"..."}`

