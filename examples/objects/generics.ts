function identity<T>(x: T): T {
  return x;
}

console.log(identity<number>(42));
console.log(identity<string>("typed"));
