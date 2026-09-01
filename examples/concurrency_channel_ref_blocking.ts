const nativeRefBlockingChannel = channel<any>(0);
const nativeRefBlockingSender = spawn((): void => {
  channelSend(nativeRefBlockingChannel, "task-ref");
});
const nativeRefBlockingReceiver = spawn((): any => channelRecv(nativeRefBlockingChannel));
console.log(join(nativeRefBlockingReceiver));
join(nativeRefBlockingSender);
