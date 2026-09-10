// expect: true

// FormData instances must inherit FormData.prototype (same bug class as
// #395's Blob instanceof fix) - undici-style isFormDataLike() checks and
// plain `instanceof FormData` both depend on this.
const fd = new FormData();
fd instanceof FormData &&
  fd.constructor === FormData &&
  Object.getPrototypeOf(fd) === FormData.prototype;
