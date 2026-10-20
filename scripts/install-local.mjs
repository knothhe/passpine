#!/usr/bin/env node
import { spawnSync } from "node:child_process"
import fs from "node:fs"
import { homedir } from "node:os"
import path from "node:path"
import process from "node:process"
import { createInterface } from "node:readline"
import { fileURLToPath, pathToFileURL } from "node:url"

const browsers = ["chrome", "firefox", "edge"]
const defaultOutputs = { chrome: "browser/.output/chrome-mv3", firefox: "browser/.output/firefox-mv3", edge: "browser/.output/chrome-mv3" }
const marker = ".h2-ext-local.json"
const projectRoot = fileURLToPath(new URL("../", import.meta.url))
const object = value => value !== null && typeof value === "object" && !Array.isArray(value)

function requireThat(condition, message) {
  if (!condition)
    throw new Error(message)
}

function parseArgs(argv) {
  const args = {}
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    if (arg === "--")
      continue
    if (["--browser", "--dir", "--name", "--config"].includes(arg)) {
      const key = arg.slice(2)
      requireThat(args[key] === undefined, `Repeated option: ${arg}`)
      const value = argv[++i]
      requireThat(value && !value.startsWith("-"), `Expected a value after ${arg}.`)
      args[key] = value
    }
    else {
      requireThat(!arg.startsWith("-") && args.browser === undefined, `Unexpected argument: ${arg}`)
      args.browser = arg
    }
  }
  return args
}

function validateName(name) {
  requireThat(typeof name === "string" && name.length > 0 && name === name.trim() && !name.endsWith(".") && !/[\p{Cc}<>:"/\\|?*]/u.test(name) && !/^(?:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(name), "Plugin name must be a single non-empty folder name without path separators, leading/trailing spaces or reserved filesystem characters.")
}

function validExtensionVersion(version, browser) {
  if (typeof version !== "string")
    return false
  const parts = version.split(".")
  // Firefox uses Mozilla's 1–4 component format (up to nine digits).
  // Chrome/Edge additionally cap components at 65535 and forbid all zero.
  const maxComponent = browser === "firefox" ? 999999999 : 65535
  return parts.length <= 4
    && parts.every(part => /^(0|[1-9][0-9]{0,8})$/.test(part) && part === String(Number(part)) && Number(part) <= maxComponent)
    && (browser === "firefox" || parts.some(part => Number(part) > 0))
}

function resolveDirectory(value, root) {
  requireThat(typeof value === "string" && value.trim(), "Installation directory must be a non-empty string.")
  const expanded = value === "~" ? homedir() : value.startsWith("~/") ? path.join(homedir(), value.slice(2)) : value
  requireThat(!expanded.startsWith("~"), "Only ~ or ~/ paths are supported for home expansion.")
  return path.resolve(root, expanded)
}

// Resolve existing ancestors too, so aliases cannot bypass overlap checks.
function canonical(filename) {
  try {
    return fs.realpathSync(filename)
  }
  catch (error) {
    if (error.code !== "ENOENT")
      throw error
    return path.join(canonical(path.dirname(filename)), path.basename(filename))
  }
}

function contains(parent, child) {
  const relative = path.relative(parent, child)
  return relative === "" || (relative !== ".." && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative))
}

function validateDestination(destination, root, browser, sources) {
  const realDestination = canonical(destination)
  const realRoot = canonical(root)
  const output = canonical(path.join(realRoot, ".output"))
  requireThat(!contains(realDestination, realRoot) && !contains(realRoot, realDestination), "Plugin directory must not overlap the repository.")
  requireThat(!contains(output, realDestination) && !contains(realDestination, output), "Installation directory must not overlap .output.")
  for (const source of [...sources, path.join(root, ".git"), path.join(root, "node_modules"), path.join(root, "scripts")]) {
    const realSource = canonical(source)
    requireThat(!contains(realSource, realDestination) && !contains(realDestination, realSource), "Installation directory must not overlap build sources or protected project directories.")
  }
  let stat
  try {
    stat = fs.lstatSync(destination)
  }
  catch (error) {
    if (error.code === "ENOENT")
      return
    throw error
  }
  requireThat(stat.isDirectory() && !stat.isSymbolicLink(), "Installation directory must be a real directory, not a symlink or file.")
  if (fs.readdirSync(destination).length === 0)
    return
  let ownership
  try {
    ownership = JSON.parse(fs.readFileSync(path.join(destination, marker), "utf8"))
  }
  catch {
    // Existing manual installations need confirmation before replacement.
  }
  return !(ownership?.installer === "h2-ext-local" && ownership?.schema === 2 && ownership?.project === realRoot && ownership?.browser === browser)
}

export async function confirmOverwrite(destination, { input = process.stdin, output = process.stdout } = {}) {
  const readline = createInterface({ input, output, terminal: Boolean(input.isTTY && output.isTTY) })
  readline.on("SIGINT", () => readline.close())
  try {
    output.write(`Plugin directory already contains files: ${destination}\nOverwriting will replace all files in this plugin directory.\nOverwrite this plugin? [Y/n] `)
    for await (const line of readline) {
      const answer = line.trim().toLowerCase()
      if (["", "y", "yes"].includes(answer))
        return true
      if (["n", "no"].includes(answer))
        return false
      output.write("Please enter Y or n. Overwrite this plugin? [Y/n] ")
    }
    // Closed stdin (including non-interactive runs) is cancellation, not Yes.
    return false
  }
  finally {
    readline.close()
  }
}

function settings(config, root) {
  const maps = ["directories", "build_scripts", "output_directories"]
  requireThat(Object.keys(config).every(key => ["browser", "name", "package_manager", ...maps].includes(key)), "Unknown local installation config field.")
  for (const key of maps)
    requireThat(config[key] === undefined || (object(config[key]) && Object.entries(config[key]).every(([browser, value]) => browsers.includes(browser) && typeof value === "string" && value.trim())), `${key} must map chrome, firefox or edge to non-empty strings.`)
  requireThat(config.browser === undefined || browsers.includes(config.browser), "Browser must be chrome, firefox or edge.")
  let packageManager = config.package_manager
  if (packageManager === undefined) {
    const filename = path.join(root, "package.json")
    const declared = fs.existsSync(filename) ? JSON.parse(fs.readFileSync(filename, "utf8")).packageManager?.split("@")[0] : undefined
    packageManager = declared ?? (fs.existsSync(path.join(root, "pnpm-lock.yaml")) ? "pnpm" : "npm")
  }
  requireThat(["npm", "pnpm"].includes(packageManager), "Built-in installer supports npm or pnpm; adapt it for this project's package manager.")
  const outputs = Object.fromEntries(browsers.map(browser => [browser, resolveDirectory(config.output_directories?.[browser] ?? defaultOutputs[browser], root)]))
  for (const output of Object.values(outputs))
    requireThat(contains(canonical(root), canonical(output)) && canonical(output) !== canonical(root), "Build output must be a dedicated directory inside the project.")
  return { packageManager, outputs, buildScripts: config.build_scripts ?? {} }
}

function build(browser, root, options) {
  const script = options.buildScripts[browser] ?? `build:browser:${browser}`
  const pkg = JSON.parse(fs.readFileSync(path.join(root, "package.json"), "utf8"))
  requireThat(typeof pkg.scripts?.[script] === "string", `Missing build script: ${script}. Configure build_scripts for ${browser}.`)
  const command = options.packageManager + (process.platform === "win32" ? ".cmd" : "")
  const result = spawnSync(command, ["run", script], { cwd: root, stdio: "inherit" })
  if (result.error)
    throw result.error
  requireThat(result.status === 0, `Build failed for ${browser}; installation was not changed.`)
}

export async function installLocal(argv, { root = projectRoot, runBuild = build, confirmReplacement = confirmOverwrite } = {}) {
  const args = parseArgs(argv)
  const configPath = path.resolve(root, args.config ?? ".install-local.json")
  let config = {}
  if (args.config !== undefined || fs.existsSync(configPath))
    config = JSON.parse(fs.readFileSync(configPath, "utf8"))
  requireThat(object(config), "Local installation config must contain an object.")
  const options = settings(config, root)
  const browser = args.browser ?? config.browser ?? "chrome"
  requireThat(browsers.includes(browser), "Browser must be chrome, firefox or edge.")
  const directory = args.dir ?? config.directories?.[browser]
  requireThat(directory !== undefined, `No installation directory configured for ${browser}. Copy .install-local.example.json to .install-local.json and edit its directories, or pass --dir <path>.`)
  const packagePath = path.join(root, "package.json")
  const packageName = fs.existsSync(packagePath) ? JSON.parse(fs.readFileSync(packagePath, "utf8")).name : undefined
  const name = args.name ?? config.name ?? packageName ?? path.basename(canonical(root))
  validateName(name)
  const parentDirectory = resolveDirectory(directory, root)
  const destination = path.join(parentDirectory, name)
  const needsConfirmation = validateDestination(destination, root, browser, Object.values(options.outputs))
  const overwriteConfirmed = needsConfirmation && await confirmReplacement(destination)
  if (needsConfirmation && !overwriteConfirmed)
    return null
  await runBuild(browser, root, options)
  const source = options.outputs[browser]
  const manifest = JSON.parse(fs.readFileSync(path.join(source, "manifest.json"), "utf8"))
  requireThat(manifest?.manifest_version === 3 && typeof manifest.name === "string" && manifest.name.trim(), "Build output must contain a valid MV3 manifest.")
  const versionRules = browser === "firefox"
    ? "1–4 integers of up to nine digits, without leading zeros"
    : "1–4 integers between 0 and 65535, without leading zeros, and must not be all zero"
  requireThat(validExtensionVersion(manifest.version, browser), `Manifest version for ${browser} must contain ${versionRules}; installation was not changed.`)
  requireThat(!validateDestination(destination, root, browser, Object.values(options.outputs)) || overwriteConfirmed, "Installation directory changed during the build and now needs overwrite confirmation. Run make browser:install again.")
  fs.mkdirSync(path.dirname(destination), { recursive: true })
  const staging = fs.mkdtempSync(path.join(path.dirname(destination), ".h2-ext-local-"))
  const stagedExtension = path.join(staging, "extension")
  const backup = path.join(staging, "previous")
  let backupNeedsRecovery = false
  try {
    fs.cpSync(source, stagedExtension, { recursive: true })
    fs.writeFileSync(path.join(stagedExtension, marker), `${JSON.stringify({ installer: "h2-ext-local", schema: 2, project: canonical(root), browser, version: manifest.version }, null, 2)}\n`)
    requireThat(!validateDestination(destination, root, browser, Object.values(options.outputs)) || overwriteConfirmed, "Installation directory changed before replacement and now needs overwrite confirmation. Run make browser:install again.")
    const hadPrevious = fs.existsSync(destination)
    if (hadPrevious) {
      fs.renameSync(destination, backup)
      backupNeedsRecovery = true
    }
    try {
      fs.renameSync(stagedExtension, destination)
      backupNeedsRecovery = false
    }
    catch (error) {
      if (hadPrevious) {
        fs.renameSync(backup, destination)
        backupNeedsRecovery = false
      }
      throw error
    }
  }
  finally {
    // If restoration failed, preserve the previous installation for recovery.
    if (backupNeedsRecovery)
      console.error(`Previous installation preserved at: ${backup}`)
    else
      fs.rmSync(staging, { recursive: true, force: true })
  }
  return { browser, destination, version: manifest.version }
}

async function main() {
  const argv = process.argv.slice(2)
  if (argv.includes("--help") || argv.includes("-h")) {
    console.log("Usage: node scripts/install-local.mjs [chrome|firefox|edge] [--browser <browser>] [--dir <parent-directory>] [--name <plugin-name>] [--config <file>]\nBuilds MV3 and installs into <parent-directory>/<plugin-name>. Default name: package.json name, falling back to the project folder name.\nDefaults: .install-local.json, then Chrome. CLI options override config.\nRelative parent directories resolve from the repository; ~/ expands to your home.\nOnly the named plugin directory is replaced; other files in the parent directory are preserved.\nExisting plugin directories from other installations require overwrite confirmation [Y/n].\nLoad the plugin directory in Chrome/Edge, or its manifest.json temporarily in Firefox. Reload after updates.")
    return
  }
  const result = await installLocal(argv)
  if (result === null) {
    console.log("Installation cancelled; directory left unchanged.")
    return
  }
  const { browser, destination, version } = result
  console.log(`Installed extension ${version} (${browser}) to ${destination}`)
  if (browser === "firefox")
    console.log(`Load ${path.join(destination, "manifest.json")} at about:debugging#/runtime/this-firefox (temporary; reload after updates).`)
  else
    console.log(`Load unpacked at ${browser}://extensions with Developer mode enabled; reload the extension after updates.`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(fs.realpathSync(process.argv[1])).href) {
  try {
    await main()
  }
  catch (error) {
    console.error(`Error: ${error.message}`)
    process.exitCode = 1
  }
}
