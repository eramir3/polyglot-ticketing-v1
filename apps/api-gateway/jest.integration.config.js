module.exports = {
  displayName: 'api-gateway-integration',
  rootDir: '../..',
  testEnvironment: 'node',
  testMatch: ['<rootDir>/apps/api-gateway/test/**/*.integration-spec.ts'],
  extensionsToTreatAsEsm: ['.ts'],
  transform: {
    '^.+\\.ts$': [
      'ts-jest',
      {
        tsconfig: '<rootDir>/apps/api-gateway/tsconfig.spec.json',
        useESM: true,
      },
    ],
  },
  moduleNameMapper: {
    '^(\\.{1,2}/.*)\\.js$': '$1',
  },
  moduleFileExtensions: ['ts', 'js'],
  maxWorkers: 1,
  testTimeout: 120000,
};
