package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

func TestLinuxAMD64TypedSpawnJoinResults(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "typed_spawn_join_results",
		source: `
const base = 40;
const numberTask = spawn((): number => base + 2);
const boolTask = spawn((): boolean => true);
const stringTask = spawn((): string => "task-" + "string");
const objectTask = spawn((): { value: number } => ({ value: 42 }));
const arrayTask = spawn((): number[] => [1, 42, 3]);
const anyTask = spawn((): any => 42);
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
console.log(join(numberTask));
console.log(join(numberTask));
console.log(join(boolTask));
console.log(join(stringTask));
console.log(join(objectTask).value);
console.log(join(arrayTask)[1]!);
console.log(join(anyTask));
`,
		expected: "42\n42\ntrue\ntask-string\n42\n42\n42\n",
	})
}

func TestLinuxAMD64SpawnBlockClosureCaptures(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "spawn_block_closure_captures",
		source: `
const message = "captured";
const task = spawn((): void => { console.log(message); });
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
join(task);
console.log("done");
`,
		expected: "captured\ndone\n",
	})
}

func TestLinuxAMD64CooperativeTaskQueueAndYield(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "cooperative_task_queue_and_yield",
		source: `
const first = spawn((): void => { console.log(1); });
const second = spawn((): void => { console.log(2); });
yieldNow();
console.log(3);
join(second);
join(first);
const outer = spawn((): number => {
  const child = spawn((): number => 40);
  return join(child) + 2;
});
console.log(join(outer));
`,
		expected: "1\n3\n2\n42\n",
	})
}
func TestLinuxAMD64TaskSleepAndDelayedJoin(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "task_sleep_and_delayed_join",
		source: `
sleep(0);
const task = spawn((): number => {
  sleep(2);
  const child = spawn((): number => {
    sleep(2);
    return 40;
  });
  sleep(1);
  return join(child) + 2;
});
console.log(join(task));
console.log(7);
`,
		expected: "42\n7\n",
	})
}
func TestLinuxAMD64BufferedChannelTryOperations(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "buffered_channel_try_operations",
		source: `
const numbers = channel<number>(1);
console.log(channelTrySend(numbers, 42));
console.log(channelTrySend(numbers, 99));
console.log(channelTryRecvOr(numbers, 7));
console.log(channelTryRecvOr(numbers, 7));
const strings = channel<string>(1);
const held = "keep-" + "alive";
console.log(channelTrySend(strings, held));
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
console.log(channelTryRecvOr(strings, "fallback"));
console.log(channelTryRecvOr(strings, "fallback"));
`,
		expected: "true\nfalse\n42\n7\ntrue\nkeep-alive\nfallback\n",
	})
}
func TestLinuxAMD64BlockingBufferedAndRendezvousChannels(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "blocking_buffered_and_rendezvous_channels",
		source: `
const buffered = channel<number>(1);
channelSend(buffered, 1);
const consumer = spawn((): void => { console.log(channelRecv(buffered)); });
channelSend(buffered, 2);
join(consumer);
console.log(channelRecv(buffered));

const rendezvous = channel<string>(0);
const held = "keep-" + "alive";
const sender = spawn((): void => { channelSend(rendezvous, held); });
const churner = spawn((): void => {
  let churn = "";
  for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
});
const receiver = spawn((): string => channelRecv(rendezvous));
console.log(join(receiver));
join(sender);
join(churner);
`,
		expected: "1\n2\nkeep-alive\n",
	})
}
func TestLinuxAMD64StackfulTaskSuspensionAndCrossChannels(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "stackful_task_suspension_and_cross_channels",
		source: `
const keeper = spawn((): void => {
  const keep = "keep-" + "alive";
  yieldNow();
  console.log(keep);
});
const churner = spawn((): void => {
  let churn = "";
  for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
});
join(keeper);
join(churner);

const ready = channel<number>(1);
const inbound = channel<string>(0);
const receiver = spawn((): string => {
  channelSend(ready, 1);
  return channelRecv(inbound);
});
channelRecv(ready);
channelSend(inbound, "external-to-task");
console.log(join(receiver));

const signal = channel<boolean>(0);
const waiter = spawn((): void => { channelRecv(signal); console.log(2); });
const producer = spawn((): void => {
  channelSend(signal, true);
  console.log(1);
  yieldNow();
  console.log(3);
});
join(waiter);
join(producer);
`,
		expected: "keep-alive\nexternal-to-task\n2\n1\n3\n",
	})
}
func TestLinuxAMD64TaskFunctionResult(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "task_function_result",
		source: `
const task = spawn((): (() => number) => {
  const inner = (): number => 42;
  return inner;
});
const fn: () => number = join(task);
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
console.log(fn());
`,
		expected: "42\n",
	})
}
func TestLinuxAMD64TaskCancellationFlag(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "task_cancellation_flag",
		source: `
const gate = channel<boolean>(0);
const worker = spawn((): void => {
  channelRecv(gate);
  console.log(taskCancelled());
});
cancelTask(worker);
const release = spawn((): void => { channelSend(gate, true); });
join(worker);
join(release);
console.log(taskCancelled());
`,
		expected: "true\nfalse\n",
	})
}
func TestLinuxAMD64TaskContextInheritance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "task_context_inheritance",
		source: `
const parent = spawn((): void => {
  setTaskContext("trace-" + "42");
  const child = spawn((): void => {
    yieldNow();
    console.log(taskContext());
  });
  let churn = "";
  for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
  join(child);
  console.log(taskContext());
});
join(parent);
`,
		expected: "trace-42\ntrace-42\n",
	})
}
func TestLinuxAMD64TaskGroups(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "task_groups",
		source: `
const group = taskGroup();
const values = channel<number>(2);
const a = groupSpawn(group, (): number => { channelSend(values, 1); return 10; });
const b = groupSpawn(group, (): number => { channelSend(values, 2); return 20; });
groupJoin(group);
console.log(channelRecv(values));
console.log(channelRecv(values));
console.log(join(a) + join(b));

const cancelled = taskGroup();
const gate = channel<number>(2);
const done = channel<number>(2);
const ca = groupSpawn(cancelled, (): void => { channelRecv(gate); if (taskCancelled()) { channelSend(done, 1); return; } });
const cb = groupSpawn(cancelled, (): void => { channelRecv(gate); if (taskCancelled()) { channelSend(done, 2); return; } });
groupCancel(cancelled);
channelSend(gate, 1);
channelSend(gate, 1);
groupJoin(cancelled);
console.log(channelRecv(done) + channelRecv(done));
join(ca);
join(cb);
`,
		expected: "1\n2\n30\n3\n",
	})
}

func TestLinuxAMD64FulfilledAsyncAwait(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "fulfilled_async_await",
		source: `
async function numberLeaf(v: number): Promise<number> { sleep(1); return v; }
async function numberParent(): Promise<number> { return await numberLeaf(42); }
async function boolLeaf(v: boolean): Promise<boolean> { yieldNow(); return v; }
async function boolParent(): Promise<boolean> { return await boolLeaf(true); }
async function stringLeaf(v: string): Promise<string> {
  yieldNow();
  let churn = "";
  for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
  return v + "-done";
}
async function stringParent(v: string): Promise<string> { return await stringLeaf(v); }
console.log(join(numberParent()));
console.log(join(boolParent()));
console.log(join(stringParent("async")));
`,
		expected: "42\ntrue\nasync-done\n",
	})
}

func TestLinuxAMD64AsyncTryCatchAndAwaitRejection(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "async_try_catch_and_await_rejection",
		source: `
async function local(): Promise<any> {
  try {
    let reason = "caught-" + "local";
    throw reason;
  } catch (error: any) {
    let churn = "";
    for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
    return error;
  }
}
async function leaf(): Promise<number> {
  yieldNow();
  throw "caught-await";
}
async function parent(): Promise<any> {
  try {
    return await leaf();
  } catch (error: any) {
    let churn = "";
    for (let i = 0; i < 50000; i = i + 1) { churn = "xy" + "zz"; }
    return error;
  }
}
console.log(join(local()));
console.log(join(parent()));
`,
		expected: "caught-local\ncaught-await\n",
	})
}

func TestLinuxAMD64AsyncFinallyCompletionSemantics(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "async_finally_completion_semantics",
		source: `
async function returnOverride(): Promise<number> {
  try { return 40; } catch (error: any) { return 1; } finally { return 99; }
}
async function throwOverride(): Promise<number> {
  try { return 40; } catch (error: any) { return 1; } finally { throw "override"; }
}
async function recoverOverride(): Promise<number> {
  try { return await throwOverride(); }
  catch (error: any) { console.log(error); return 42; }
  finally { console.log("outer-finally"); }
}
async function nested(): Promise<number> {
  try {
    try { throw "inner"; }
    catch (error: any) { console.log(error); throw "middle"; }
    finally { console.log("inner-finally"); }
  } catch (error: any) { console.log(error); return 7; }
  finally { console.log("final-finally"); }
}
console.log(join(returnOverride()));
console.log(join(recoverOverride()));
console.log(join(nested()));
`,
		expected: "99\noverride\nouter-finally\n42\ninner\ninner-finally\nmiddle\nfinal-finally\n7\n",
	})
}

func TestLinuxAMD64PromiseResolveRejectAdoption(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "promise_resolve_reject_adoption",
		source: `
async function probe(): Promise<string> {
  const original: Promise<string> = Promise.resolve("root-" + "value");
  const adopted: Promise<string> = Promise.resolve(original);
  const first: string = await original;
  let churn = "";
  for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
  const second: string = await adopted;
  return first + ":" + second;
}
async function rejected(): Promise<number> {
  try { return await Promise.reject<number>("reject-root"); }
  catch (error: any) { console.log(error); return 42; }
}
async function repeated(): Promise<number> {
  let p: Promise<number> = Promise.resolve(21);
  const a = await p;
  p = Promise.resolve(p);
  const b = await p;
  return a + b;
}
console.log(join(probe()));
console.log(join(rejected()));
console.log(join(repeated()));
`,
		expected: "root-value:root-value\nreject-root\n42\n42\n",
	})
}

func TestLinuxAMD64DrainsFireAndForgetTasksBeforeExit(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "drain_fire_and_forget_tasks",
		source: `
async function fire(): Promise<void> {
  yieldNow();
  console.log(42);
}
fire();
console.log(1);
`,
		expected: "1\n42\n",
	})
}

func TestLinuxAMD64ClassMethodUnusedTrailingArgumentPreservesThis(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "class_method_unused_trailing_arg_preserves_this",
		source: `
class Box {
  value: number;
  constructor(value: number) { this.value = value; }
  invoke(resolve: (value: number) => void, unused: (reason: any) => void): void {
    resolve(this.value + 2);
  }
}
const box = new Box(40);
box.invoke((value: number): void => { console.log(value); }, (reason: any): void => {});
`,
		expected: "42\n",
	})
}

func TestLinuxAMD64PromiseThenableAssimilation(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "promise_thenable_assimilation",
		source: `
interface NumberThenable {
  base: number;
  then: (this: NumberThenable, resolve: (value: number) => void, reject: (reason: any) => void) => void;
}

class BaseThenable {
  value: number;
  constructor(value: number) { this.value = value; }
  then(resolve: (value: number) => void, reject: (reason: any) => void): void { resolve(this.value); }
}
class DerivedThenable extends BaseThenable {
  override then(resolve: (value: number) => void, reject: (reason: any) => void): void { resolve(this.value + 2); }
}

async function structural(): Promise<number> {
  const value: NumberThenable = {
    base: 40,
    then: function (this: NumberThenable, resolve: (value: number) => void, reject: (reason: any) => void): void {
      resolve(this.base + 2);
      reject("late");
    },
  };
  return await Promise.resolve(value);
}

async function delayed(): Promise<number> {
  const value: NumberThenable = {
    base: 40,
    then: function (this: NumberThenable, resolve: (value: number) => void, reject: (reason: any) => void): void {
      const result = this.base + 2;
      spawn((): void => { sleep(1); resolve(result); reject("late-async"); });
    },
  };
  return await Promise.resolve(value);
}

async function classValue(): Promise<number> {
  const value: BaseThenable = new DerivedThenable(40);
  return await Promise.resolve(value);
}

async function rejected(): Promise<number> {
  const value: NumberThenable = {
    base: 0,
    then: function (this: NumberThenable, resolve: (value: number) => void, reject: (reason: any) => void): void {
      reject("thenable-reject");
      resolve(99);
    },
  };
  try {
    return await Promise.resolve(value);
  } catch (error: any) {
    console.log(error);
    return 7;
  }
}

console.log(join(structural()));
console.log(join(delayed()));
console.log(join(classValue()));
console.log(join(rejected()));
`,
		expected: "42\n42\n42\nthenable-reject\n7\n",
	})
}

func TestLinuxAMD64SchedulerAwareSleepOrdering(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "scheduler_aware_sleep_ordering",
		source: `
const out = channel<number>(2);
spawn((): void => { sleep(8); channelSend(out, 20); });
spawn((): void => { sleep(1); channelSend(out, 30); });
console.log(channelRecv(out));
console.log(channelRecv(out));
`,
		expected: "30\n20\n",
	})
}
func TestLinuxAMD64PromiseLiteralAggregates(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "promise_literal_aggregates",
		source: `
async function delayed(ms: number, value: number): Promise<number> {
  sleep(ms);
  return value;
}
async function allValues(): Promise<number> {
  const values = await Promise.all<number>([delayed(8, 1), Promise.resolve(2), 3]);
  return values[0]! * 100 + values[1]! * 10 + values[2]!;
}
async function rejectEarly(): Promise<number> {
  try {
    await Promise.all<number>([delayed(8, 1), Promise.reject<number>("aggregate-boom"), delayed(1, 3)]);
    return 0;
  } catch (error: any) {
    console.log(error);
    return 42;
  }
}
async function raceValues(): Promise<number> {
  return await Promise.race<number>([delayed(8, 20), delayed(1, 30)]);
}
console.log(join(allValues()));
console.log(join(rejectEarly()));
console.log(join(raceValues()));
`,
		expected: "123\naggregate-boom\n42\n30\n",
	})
}
func TestLinuxAMD64PromiseArrayVariableAggregates(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "promise_array_variable_aggregates",
		source: `
async function delayed(ms: number, value: number): Promise<number> {
  sleep(ms);
  return value;
}
async function allFromArray(): Promise<number> {
  const first = delayed(4, 10);
  const second = Promise.resolve(20);
  const items = [first, second];
  const values = await Promise.all(items);
  return values[0]! + values[1]!;
}
async function raceFromArray(): Promise<number> {
  const slow = delayed(8, 20);
  const fast = delayed(1, 30);
  const items = [slow, fast];
  return await Promise.race(items);
}
async function rejectFromArray(): Promise<number> {
  try {
    const slow = delayed(8, 1);
    const bad = Promise.reject<number>("array-boom");
    const items = [slow, bad];
    await Promise.all(items);
    return 0;
  } catch (error: any) {
    console.log(error);
    return 42;
  }
}
console.log(join(allFromArray()));
console.log(join(raceFromArray()));
console.log(join(rejectFromArray()));
`,
		expected: "30\n30\narray-boom\n42\n",
	})
}
func TestLinuxAMD64PromiseLikeClassImplementsAndGuardNarrowing(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "promise_like_class_implements_guard_narrowing",
		source: `
class StandardPromiseLike implements PromiseLike<number> {
  value: number;
  constructor(value: number) { this.value = value; }
  then(
    onfulfilled?: ((value: number) => any) | null,
    onrejected?: ((reason: any) => any) | null,
  ): any {
    if (onfulfilled === undefined) { return this; }
    if (onfulfilled === null) { return this; }
    onfulfilled(this.value + 2);
    return this;
  }
}
async function run(): Promise<number> {
  return await Promise.resolve(new StandardPromiseLike(40));
}
console.log(join(run()));
`,
		expected: "42\n",
	})
}
func TestLinuxAMD64BooleanConsoleMatchesJavaScript(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "boolean_console_matches_javascript",
		source: `
console.log(true);
console.log(false);
`,
		expected: "true\nfalse\n",
	})
}

func TestLinuxAMD64StringConcatFusionSemantics(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "string_concat_fusion",
		source: `
console.log("a" + 1 + true + "z");
console.log(1 + 2 + "x");
`,
		expected: "a1truez\n3x\n",
	})
}

func TestLinuxAMD64CompletedTaskStacksAreUnmapped(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("native Linux AMD64 execution required")
	}
	const src = `
let total = 0;
for (let i = 0; i < 256; i = i + 1) {
  const task = spawn((): number => 1);
  total = total + join(task);
}
console.log(total);
`
	dir := t.TempDir()
	binPath := filepath.Join(dir, "task-stack-reclaim")
	compiler := tspro.New(tspro.Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	bin, diags, err := compiler.CompileSource("task-stack-reclaim.ts", []byte(src))
	if err != nil {
		t.Fatalf("compile: %v, diagnostics: %s", err, diags.Format(compiler.FileSet()))
	}
	if err := os.WriteFile(binPath, bin, 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	// Each task reserves a 1 MiB private stack. Without reclamation, 256
	// sequential tasks exceed this address-space limit even though only one task
	// is live at a time.
	cmd := exec.Command("bash", "-c", "ulimit -v 131072; exec \"$1\"", "bash", binPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("limited-address-space execution failed: %v\nOutput:\n%s", err, out)
	}
	if string(out) != "256\n" {
		t.Fatalf("stdout: got %q, want %q", out, "256\\n")
	}
}
