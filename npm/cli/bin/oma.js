#!/usr/bin/env node
const { execFileSync } = require('child_process');
const path = require('path');
const os = require('os');

const platformMap = {
  'darwin-arm64': '@oma/cli-darwin-arm64',
  'darwin-x64': '@oma/cli-darwin-x64',
  'linux-x64': '@oma/cli-linux-x64',
  'linux-arm64': '@oma/cli-linux-arm64',
};

const platform = `${os.platform()}-${os.arch()}`;
const pkg = platformMap[platform];

if (!pkg) {
  console.error(`Unsupported platform: ${platform}`);
  process.exit(1);
}

try {
  const binPath = require.resolve(`${pkg}/oma`);
  execFileSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  console.error(`Failed to find binary for ${platform}. Try reinstalling @oma/cli.`);
  process.exit(1);
}
