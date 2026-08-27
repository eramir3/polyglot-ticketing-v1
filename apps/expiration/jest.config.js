module.exports = {
  displayName: 'expiration',
  rootDir: '../..',
  testEnvironment: 'node',
  testMatch: ['<rootDir>/apps/expiration/src/**/*.spec.ts'],
  extensionsToTreatAsEsm: ['.ts'],
  transform: {
    '^.+\\.ts$': [
      'ts-jest',
      {
        tsconfig: '<rootDir>/apps/expiration/tsconfig.spec.json',
        useESM: true,
      },
    ],
  },
  moduleNameMapper: {
    '^(\\.{1,2}/.*)\\.js$': '$1',
  },
  moduleFileExtensions: ['ts', 'js'],
  maxWorkers: 1,
};
