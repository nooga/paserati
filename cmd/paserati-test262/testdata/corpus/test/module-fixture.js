/*---
description: A module test importing a fixture statically and dynamically; must pass however -path is spelled.
flags: [module, async]
---*/
import { value } from './module-fixture_FIXTURE.js';

assert.sameValue(value, 42);
import('./module-fixture_FIXTURE.js')
  .then(function (ns) { assert.sameValue(ns.value, 42); })
  .then($DONE, $DONE);
