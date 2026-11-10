const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')

const source = fs.readFileSync(path.join(__dirname, '../assets/js/resize.js'), 'utf8')
const start = source.indexOf('  function resizeDelta(')
const end = source.indexOf('\n  }', start)
assert(start >= 0 && end > start, 'resizeDelta not found')
const context = vm.createContext({})
vm.runInContext(source.slice(start, end + 4), context)
const resizeDelta = context.resizeDelta

assert.equal(resizeDelta('x', 100, 0, 140, 0, false), 40, 'LTR grows with a rightward drag')
assert.equal(resizeDelta('x', 100, 0, 60, 0, false), -40, 'LTR shrinks with a leftward drag')
assert.equal(resizeDelta('x', 100, 0, 60, 0, true), 40, 'RTL grows with a leftward drag')
assert.equal(resizeDelta('x', 100, 0, 140, 0, true), -40, 'RTL shrinks with a rightward drag')
assert.equal(resizeDelta('y', 0, 100, 140, 70, true), -30, 'Vertical resizing ignores direction')

console.log('Resize direction: LTR and RTL horizontal drags, and vertical drags passed.')
