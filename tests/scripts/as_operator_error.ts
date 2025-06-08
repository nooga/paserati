// expect_compile_error: conversion of type

// Test invalid type assertion that should produce a compile error
let x: string = "hello";
let num = x as number;  // This should error
num;