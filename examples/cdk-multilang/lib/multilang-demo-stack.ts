import fs from 'node:fs';
import path from 'node:path';
import { execSync } from 'node:child_process';

import { CfnOutput, Duration, RemovalPolicy, Stack, type StackProps } from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import { NodejsFunction } from 'aws-cdk-lib/aws-lambda-nodejs';
import { Construct } from 'constructs';

function repoRootFrom(stackFileDir: string): string {
  return path.resolve(stackFileDir, '../../..');
}

export class MultilangDemoStack extends Stack {
  constructor(scope: Construct, id: string, props: StackProps = {}) {
    super(scope, id, props);

    const repoRoot = repoRootFrom(__dirname);

    const table = new dynamodb.Table(this, 'DemoTable', {
      partitionKey: { name: 'PK', type: dynamodb.AttributeType.STRING },
      sortKey: { name: 'SK', type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      removalPolicy: RemovalPolicy.DESTROY,
    });

    const goHandlerDir = path.join(repoRoot, 'examples/cdk-multilang/lambdas/go');
    const goFn = new lambda.Function(this, 'GoDemoFn', {
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.X86_64,
      handler: 'bootstrap',
      memorySize: 256,
      timeout: Duration.seconds(10),
      code: lambda.Code.fromAsset(goHandlerDir, {
        bundling: {
          image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
          command: [
            'bash',
            '-c',
            [
              'set -euo pipefail',
              `cd /asset-input`,
              'GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /asset-output/bootstrap .',
            ].join('\n'),
          ],
          local: {
            tryBundle(outputDir: string): boolean {
              execSync(
                [
                  'GOOS=linux',
                  'GOARCH=amd64',
                  'CGO_ENABLED=0',
                  'go build',
                  `-o ${path.join(outputDir, 'bootstrap')}`,
                  './examples/cdk-multilang/lambdas/go',
                ].join(' '),
                { cwd: repoRoot, stdio: 'inherit' },
              );
              return true;
            },
          },
        },
      }),
      environment: {
        TABLE_NAME: table.tableName,
      },
    });

    const nodeEntry = path.join(repoRoot, 'examples/cdk-multilang/lambdas/node/handler.ts');
    const nodeFn = new NodejsFunction(this, 'NodeDemoFn', {
      runtime: lambda.Runtime.NODEJS_24_X,
      architecture: lambda.Architecture.X86_64,
      entry: nodeEntry,
      handler: 'handler',
      memorySize: 256,
      timeout: Duration.seconds(10),
      bundling: {
        target: 'node24',
      },
      environment: {
        TABLE_NAME: table.tableName,
      },
    });

    const pyHandlerDir = path.join(repoRoot, 'examples/cdk-multilang/lambdas/python');
    const dynamormPySrc = path.join(repoRoot, 'py/src/dynamorm_py');
    const pyFn = new lambda.Function(this, 'PythonDemoFn', {
      runtime: lambda.Runtime.PYTHON_3_14,
      architecture: lambda.Architecture.X86_64,
      handler: 'handler.handler',
      memorySize: 256,
      timeout: Duration.seconds(10),
      code: lambda.Code.fromAsset(pyHandlerDir, {
        bundling: {
          image: lambda.Runtime.PYTHON_3_12.bundlingImage,
          command: [
            'bash',
            '-c',
            [
              'set -euo pipefail',
              'cp -R /asset-input/* /asset-output/',
              'cp -R /dynamorm_py /asset-output/dynamorm_py',
            ].join('\n'),
          ],
          local: {
            tryBundle(outputDir: string): boolean {
              fs.cpSync(pyHandlerDir, outputDir, { recursive: true });
              fs.cpSync(dynamormPySrc, path.join(outputDir, 'dynamorm_py'), {
                recursive: true,
              });
              return true;
            },
          },
          volumes: [{ hostPath: dynamormPySrc, containerPath: '/dynamorm_py' }],
        },
      }),
      environment: {
        TABLE_NAME: table.tableName,
      },
    });

    table.grantReadWriteData(goFn);
    table.grantReadWriteData(nodeFn);
    table.grantReadWriteData(pyFn);

    const goUrl = goFn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.NONE });
    const nodeUrl = nodeFn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.NONE });
    const pyUrl = pyFn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.NONE });

    new CfnOutput(this, 'TableName', { value: table.tableName });
    new CfnOutput(this, 'GoFunctionUrl', { value: goUrl.url });
    new CfnOutput(this, 'NodeFunctionUrl', { value: nodeUrl.url });
    new CfnOutput(this, 'PythonFunctionUrl', { value: pyUrl.url });
  }
}
