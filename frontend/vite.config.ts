import { execSync } from 'node:child_process'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vite'

// Meld's version as the menu bar shows it: the commit the page was built from (its short hash, with
// "-dirty" for uncommitted changes) and that commit's date, or "dev" outside a git checkout.
function meldVersion(): string {
	try {
		const git = (args: string) =>
			execSync(`git ${args}`, { stdio: ['ignore', 'pipe', 'ignore'] })
				.toString()
				.trim()
		return `${git('describe --always --dirty --abbrev=7')} · ${git('log -1 --format=%cs')}`
	} catch {
		return 'dev'
	}
}

// https://vite.dev/config/
export default defineConfig({
	plugins: [svelte()],
	define: { __MELD_VERSION__: JSON.stringify(meldVersion()) },
	// make dev and make dev-page (wails3 dev) give the port, which the app's FRONTEND_DEVSERVER_URL names.
	// IPv4, since Wails' proxy to it dials localhost over IPv4 only, and Vite's localhost may be IPv6 alone.
	server: {
		host: '127.0.0.1',
		port: Number(process.env.WAILS_VITE_PORT) || 9245,
		strictPort: true,
	},
	build: {
		target: 'es2022',
		// Desktop app loading from disk. three.js is most of the start-up bundle and is needed at once,
		// so it gets its own named chunk (fetched in parallel with the app chunk). The five editors and
		// the recast WASM load on first use (App.svelte, navBuild.ts).
		chunkSizeWarningLimit: 1000,
		rolldownOptions: {
			output: {
				codeSplitting: {
					groups: [{ name: 'three', test: /node_modules[\/]three[\/]/ }],
				},
			},
		},
	},
})
