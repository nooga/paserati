try { delete unresolvable.x; } catch (e) { console.log(e.constructor.name + ": " + e.message); }
