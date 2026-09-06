console.log(btoa("hello"));
console.log(btoa("ÿ"));
console.log(atob("aGVsbG8="));
console.log(atob("YQ"));
console.log(atob(" YW Jj\n"));
console.log(atob("/w=="));
try {
  console.log(btoa("✓"));
} catch (err: any) {
  console.log(err.name);
}
try {
  console.log(atob("A"));
} catch (err: any) {
  console.log(err.name);
}
