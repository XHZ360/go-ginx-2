import type { CodegenConfig } from '@graphql-codegen/cli';

const config: CodegenConfig = {
  schema: 'src/graphql/schema.json',
  documents: 'src/graphql/**/*.graphql',
  generates: {
    'src/graphql/generated.ts': {
      plugins: ['typescript-operations'],
      config: {
        avoidOptionals: {
          variableValue: true,
          inputValue: false,
          defaultValue: false,
        },
        inputMaybeValue: 'T | null | undefined',
        maybeValue: 'T',
        onlyOperationTypes: true,
        skipTypename: true,
        useTypeImports: true,
        scalars: {
          Boolean: { input: 'boolean', output: 'boolean' },
          ID: { input: 'string', output: 'string' },
          Int: { input: 'number', output: 'number' },
          String: { input: 'string', output: 'string' },
        },
        strictScalars: true,
      },
    },
  },
};

export default config;
