export function pushPopBoolScore(b: boolean): number {
  if (b) return 1;
  return 0;
}

// Test number array push and pop
const nums: number[] = [10, 20];
console.log(nums.length);
const newLen = nums.push(30);
console.log(newLen);
console.log(nums.length);
console.log(nums[2]!);

const popped = nums.pop()!;
console.log(popped);
console.log(nums.length);

// Test push in a loop
for (let i = 0; i < 3; i++) {
  nums.push(100 + i);
}
console.log(nums.length);
console.log(nums[2]!);
console.log(nums[3]!);
console.log(nums[4]!);

// Test string array push and pop
const strs: string[] = ["alpha", "beta"];
strs.push("gamma");
console.log(strs.length);
console.log(strs[2]!);
const lastStr = strs.pop()!;
console.log(lastStr);
console.log(strs.length);

// Test boolean array push and pop
const bools: boolean[] = [true];
bools.push(false);
console.log(bools.length);
console.log(pushPopBoolScore(bools[1]!));
const lastBool = bools.pop()!;
console.log(pushPopBoolScore(lastBool));
console.log(bools.length);

// Test object array push and pop
type ArrayPushPopItem = { id: number; name: string };
const items: ArrayPushPopItem[] = [{ id: 1, name: "first" }];
items.push({ id: 2, name: "second" });
console.log(items.length);
console.log(items[1]!.name);
const lastItem = items.pop()!;
console.log(lastItem.id);
console.log(items.length);
