const os = require('os');
const path = require('path');

const platformMap = {
  'darwin-arm64': '@oma/cli-darwin-arm64',
  'darwin-x64': '@oma/cli-darwin-x64',
  'linux-x64': '@oma/cli-linux-x64',
  'linux-arm64': '@oma/cli-linux-arm64',
};

const platform = `${os.platform()}-${os.arch()}`;
const pkg = platformMap[platform];

if (!pkg) {
  console.warn(`Warning: @oma/cli does not support ${platform}. Supported platforms: ${Object.keys(platformMap).join(', ')}`);
  process.exit(0);
}

try {
  require.resolve(`${pkg}/oma`);
  console.log(`@oma/cli: platform binary found (${pkg})`);
} catch (e) {
  console.warn(`Warning: platform binary not found for ${platform}. The "oma" command may not work.`);
  console.warn(`Try: npm install ${pkg}`);
}
