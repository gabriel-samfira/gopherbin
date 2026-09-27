// Boots a real gopherbin server (fresh SQLite database, no mocks) for the
// integration suite and bootstraps the superuser through the public
// first-run endpoint, exactly like a fresh installation.

import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { mkdirSync, mkdtempSync, openSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const APP_ROOT = path.resolve(HERE, '..', '..');
const REPO_ROOT = path.resolve(APP_ROOT, '..', '..');
export const HARNESS_FILE = path.join(APP_ROOT, '.vitest-harness.json');

export const USER_PASSWORD = 'correct-horse-battery-staple-2';

const ADMIN = {
	email: 'harness@gopherbin.test',
	username: 'harnessadmin',
	full_name: 'Harness Admin',
	password: 'correct-horse-battery-staple-1',
	is_admin: true,
	enabled: true
};

export interface Harness {
	base: string;
	admin: { username: string; password: string; token: string };
}

let child: ChildProcess | undefined;
let serverDir: string | undefined;
let logPath: string | undefined;
let shuttingDown = false;

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function freePort(): Promise<number> {
	return new Promise((resolve, reject) => {
		const srv = net.createServer();
		srv.once('error', reject);
		srv.listen(0, '127.0.0.1', () => {
			const addr = srv.address() as net.AddressInfo;
			srv.close(() => resolve(addr.port));
		});
	});
}

async function stopServer(): Promise<void> {
	if (!child || child.exitCode !== null) return;
	shuttingDown = true;
	const killed = new Promise<void>((resolve) => {
		child!.once('exit', () => resolve());
	});
	child.kill('SIGTERM');
	const timeout = new Promise<void>((resolve) => setTimeout(resolve, 5000));
	await Promise.race([killed, timeout]);
	if (child.exitCode === null) {
		child.kill('SIGKILL');
	}
}

function dumpLog(): void {
	if (!logPath) return;
	try {
		const contents = readFileSync(logPath, 'utf8');
		// Keep a copy for post-mortem inspection outside the temp dir.
		try {
			writeFileSync(path.join(APP_ROOT, '.vitest-bin', 'last-server.log'), contents);
		} catch {
			/* best effort */
		}
		console.error(`--- gopherbin server log (tail) ---\n${contents.split('\n').slice(-40).join('\n')}`);
	} catch {
		/* log already gone */
	}
}

export async function setup(): Promise<void> {
	const binDir = path.join(APP_ROOT, '.vitest-bin');
	mkdirSync(binDir, { recursive: true });
	const bin = path.join(binDir, 'gopherbin');
	// The webui tag is not needed: the tests exercise the JSON API and the
	// compiled components directly, not the embedded static assets.
	execFileSync('go', ['build', '-mod', 'vendor', '-tags', 'fts5', '-o', bin, './cmd/gopherbin'], {
		cwd: REPO_ROOT,
		stdio: 'inherit'
	});

	serverDir = mkdtempSync(path.join(os.tmpdir(), 'gopherbin-vitest-'));
	const port = await freePort();
	const cfgPath = path.join(serverDir, 'server.toml');
	writeFileSync(
		cfgPath,
		[
			'[apiserver]',
			'bind = "127.0.0.1"',
			`port = ${port}`,
			'',
			'[apiserver.jwt_auth]',
			'secret = "vitest-integration-secret"',
			'time_to_live = "1h"',
			'',
			'[database]',
			'backend = "sqlite3"',
			'',
			'[database.sqlite3]',
			`db_file = "${path.join(serverDir, 'test.db').replaceAll('\\', '/')}"`,
			''
		].join('\n')
	);

	logPath = path.join(serverDir, 'server.log');
	const logFd = openSync(logPath, 'w');
	child = spawn(bin, ['-config', cfgPath], { stdio: ['ignore', logFd, logFd] });

	const base = `http://127.0.0.1:${port}/api/v1`;
	let adminToken = '';
	const deadline = Date.now() + 60_000;
	while (Date.now() < deadline) {
		if (child.exitCode !== null) break;
		let firstRun: Response;
		try {
			firstRun = await fetch(`${base}/first-run/`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(ADMIN)
			});
		} catch {
			// server not accepting connections yet
			await sleep(250);
			continue;
		}
		if (!firstRun.ok && firstRun.status !== 409) {
			// A 4xx/5xx with a body means the server is up but rejected
			// the bootstrap: retrying cannot help.
			dumpLog();
			await stopServer();
			throw new Error(`first-run returned ${firstRun.status}: ${await firstRun.text()}`);
		}
		const login = await fetch(`${base}/auth/login`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ username: ADMIN.username, password: ADMIN.password })
		});
		if (login.ok) {
			adminToken = (await login.json()).token;
			break;
		}
		await sleep(250);
	}

	if (!adminToken) {
		dumpLog();
		await stopServer();
		throw new Error('gopherbin test server did not become ready');
	}

	// Surface mid-run crashes (and keep the log around for post-mortems).
	// The server exits with a non-zero status after a clean SIGTERM too.
	child.on('exit', (code, signal) => {
		if (!shuttingDown && code !== 0 && signal !== 'SIGTERM' && signal !== 'SIGINT') {
			dumpLog();
			console.error(`gopherbin test server died unexpectedly: code=${code} signal=${signal}`);
		}
	});

	writeFileSync(
		HARNESS_FILE,
		JSON.stringify({
			base,
			admin: { username: ADMIN.username, password: ADMIN.password, token: adminToken }
		} satisfies Harness)
	);
}

export async function teardown(): Promise<void> {
	await stopServer();
	rmSync(HARNESS_FILE, { force: true });
	if (serverDir) rmSync(serverDir, { recursive: true, force: true });
}

export default async function gopherbinHarness(): Promise<() => Promise<void>> {
	await setup();
	return teardown;
}
