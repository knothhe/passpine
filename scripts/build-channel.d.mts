export type BuildChannel = 'dev' | 'release';
export function buildChannel(env?: NodeJS.ProcessEnv): BuildChannel;
export function assertReleaseBuild(env?: NodeJS.ProcessEnv, root?: string): { version: string; tag: string; head: string };
export interface ClientBuildConfig {
  schema: 1;
  channel: BuildChannel;
  version: string;
  baseVersion: string;
  commit: string;
  modified: string;
  executor: string;
  outputDirectory: string;
  appName: string;
  bundleID: string;
  packageName: string;
  linuxBinary: string;
  linuxHost: string;
  buildTags: string;
  ldflags: string;
  hostLdflags: string;
}
export function clientBuildConfig(env?: NodeJS.ProcessEnv, root?: string): ClientBuildConfig;
