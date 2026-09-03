export function swap(a: number, b: number): [number, number] {
  const [first, second] = [b, a];
  return [first, second];
}

export function getPoint(): [number, number] {
  return [100, 200];
}

// 1. Array destructuring
const numbers = [10, 20];
const [a, b] = numbers;
console.log(a);
console.log(b);

// 2. Object destructuring (with renaming)
const config = { host: "localhost", portNum: 8080 };
const { host, portNum: p } = config;
console.log(host);
console.log(p);

// 3. Tuples with heterogeneous types
const userTuple: [number, string, boolean] = [1, "alice", true];
const [userId, userName, isActive] = userTuple;
console.log(userId);
console.log(userName);
console.log(isActive);

// 4. Destructuring function return values
const [px, py] = getPoint();
console.log(px);
console.log(py);

// 5. Function internal swap
const [s1, s2] = swap(42, 99);
console.log(s1);
console.log(s2);
