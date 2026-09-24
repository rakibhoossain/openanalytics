import { defineConfig } from 'tsup';

export default defineConfig([
  // Library build (npm package) — cjs + esm + dts
  {
    entry: ['index.ts'],
    format: ['cjs', 'esm'],
    dts: true,
    splitting: true,
    sourcemap: false,
    clean: true,
    minify: true,
  },
  // IIFE build (standalone script tag)
  {
    entry: { 'src/tracker': 'src/tracker.ts' },
    format: ['iife'],
    splitting: false,
    sourcemap: false,
    minify: true,
    define: {
      __OPENANALYTICS_REPLAY_URL__: JSON.stringify(
        '/oa-replay.js'
      ),
    },
    esbuildPlugins: [
      {
        name: 'exclude-replay-from-iife',
        setup(build) {
          build.onResolve(
            { filter: /[/\\]replay([/\\]index)?(\.[jt]s)?$/ },
            () => ({
              path: 'replay-empty-stub',
              namespace: 'replay-stub',
            })
          );
          build.onLoad({ filter: /.*/, namespace: 'replay-stub' }, () => ({
            contents: 'module.exports = {}',
            loader: 'js',
          }));
        },
      },
    ],
  },
  // Replay module — built as both ESM (npm) and IIFE
  {
    entry: { 'src/replay': 'src/replay/index.ts' },
    format: ['esm', 'iife'],
    globalName: '__openanalytics_replay',
    splitting: false,
    sourcemap: false,
    minify: true,
    noExternal: ['rrweb', '@rrweb/types'],
  },
]);
