'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const test = require('node:test');

function renderer(pathname) {
  const context = vm.createContext({ window: { location: { pathname } } });
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'account-menu.js'), 'utf8'), context);
  return context;
}

test('legacy URL routing retains decoded conversation ownership', () => {
  const context = renderer('/local/thread%20one');
  assert.equal(context.codexMuxThreadIdFromLocation(), 'thread one');
});

test('memory-router navigation updates task ownership without a URL change', () => {
  const context = renderer('/index.html');
  assert.equal(context.codexMuxThreadIdFromLocation(), null);
  context.__codexMuxNativeThreadId = 'first-thread';
  assert.equal(context.codexMuxThreadIdFromLocation(), 'first-thread');
  context.__codexMuxNativeThreadId = 'second-thread';
  assert.equal(context.codexMuxThreadIdFromLocation(), 'second-thread');
  context.__codexMuxNativeThreadId = null;
  assert.equal(context.codexMuxThreadIdFromLocation(), null);
});

test('native home state does not retain a stale legacy URL thread', () => {
  const context = renderer('/local/stale-thread');
  context.__codexMuxNativeThreadId = null;
  assert.equal(context.codexMuxThreadIdFromLocation(), null);
});

test('Primary reset operations retain native authentication and secondary operations stay scoped', async () => {
  const context = renderer('/index.html');
  context.codexMuxNativeRateLimitResets = async () => ({ available_count: 2 });
  let nativeInput;
  context.codexMuxNativeConsumeRateLimitReset = async (input) => { nativeInput = input; return { code: 'reset' }; };
  const routes = [];
  context.fetch = async (url) => { routes.push(url); return { ok: true, json: async () => ({ available_count: 1 }) }; };
  assert.equal((await context.codexMuxRateLimitResets('primary')).available_count, 2);
  const input = { creditId: 'test-credit', redeemRequestId: 'test-request' };
  await context.codexMuxConsumeRateLimitReset('primary', input);
  assert.equal(nativeInput, input);
  assert.equal(routes.length, 0);
  await context.codexMuxRateLimitResets('secondary');
  assert.match(routes[0], /\/accounts\/secondary\/rate-limit-resets$/);
});
