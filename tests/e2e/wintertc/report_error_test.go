package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCReportErrorCallsGlobalOnError(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_report_error_onerror",
		source: `
globalThis.onerror = (
  message: string,
  source: string,
  line: number,
  column: number,
  error: any,
): any => {
  console.log(message);
  console.log(source === "");
  console.log(line);
  console.log(column);
  console.log(error);
  return true;
};

reportError("boom");
globalThis.onerror = null;
reportError("ignored");
console.log("done");
`,
		expected: "boom\ntrue\n0\n0\nboom\ndone\n",
	})
}
