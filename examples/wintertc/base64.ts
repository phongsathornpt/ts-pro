console.log(btoa("hello"));
console.log(btoa("ÿ"));
try {
  console.log(btoa("✓"));
} catch (err: any) {
  console.log(err.name);
}
