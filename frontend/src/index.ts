import { dialWebRTC } from "@viamrobotics/rpc";
import { GrpcWebImpl, StreamService, StreamServiceClientImpl } from "./gen/proto/stream/v1/stream";

const signalingAddress = `${window.location.protocol}//${window.location.host}`;
const host = "local";

declare global {
	interface Window {
		allowSendAudio: boolean;
	}
}

async function startup() {
	const webRTCConn = await dialWebRTC(signalingAddress, host);
	const streamClient = new StreamServiceClientImpl(new GrpcWebImpl(host, { transport: webRTCConn.transportFactory }));

	const namesResp = await streamClient.ListStreams({});

	const makeButtonClick = (button: HTMLButtonElement, streamName: string, add: boolean) => async (e: Event) => {
		e.preventDefault();

		button.disabled = true;

		if (add) {
			try {
				await streamClient.AddStream({ name: streamName });
			} catch (err) {
				console.error(err);
				button.disabled = false;
			}
		} else {
			try {
				await streamClient.RemoveStream({ name: streamName });
			} catch (err) {
				console.error(err);
				button.disabled = false;
			}
		}
	};

	webRTCConn.peerConnection.ontrack = async event => {
		const mediaElementContainer = document.createElement('div');
		mediaElementContainer.id = event.track.id;
		const mediaElement = document.createElement(event.track.kind);
		if (mediaElement instanceof HTMLVideoElement || mediaElement instanceof HTMLAudioElement) {
			mediaElement.srcObject = event.streams[0];
			mediaElement.autoplay = true;
			if (mediaElement instanceof HTMLVideoElement) {
				mediaElement.playsInline = true;				
				mediaElement.controls = false;
			} else {
				mediaElement.controls = true;
			}
		}

		const stream = event.streams[0];
		const streamName = stream.id;
		const streamContainer = document.getElementById(`stream-${streamName}`)!;
		let btns = streamContainer.getElementsByTagName("button");
		if (btns.length) {
			const button = btns[0];
			button.innerText = `Stop ${streamName}`;
			button.onclick = makeButtonClick(button, streamName, false);
			button.disabled = false;

			let audioSender: RTCRtpSender;
			stream.onremovetrack = async event => {
				const mediaElementContainer = document.getElementById(event.track.id)!;
				const mediaElement = mediaElementContainer.getElementsByTagName(event.track.kind)[0];
				if (audioSender) {
					webRTCConn.peerConnection.removeTrack(audioSender);
				}
				if (mediaElement instanceof HTMLVideoElement || mediaElement instanceof HTMLAudioElement) {
					mediaElement.pause();
					mediaElement.removeAttribute('srcObject');
					mediaElement.removeAttribute('src');
					mediaElement.load();
				}
				mediaElementContainer.remove();

				button.innerText = `Start ${streamName}`
				button.onclick = makeButtonClick(button, streamName, true);
				button.disabled = false;
			};

			if (mediaElement instanceof HTMLAudioElement && window.allowSendAudio) {
				const button = document.createElement("button");
				button.innerText = `Send audio`
				button.onclick = async (e) => {
					e.preventDefault();

					button.remove();

					navigator.mediaDevices.getUserMedia({
						audio: {
							deviceId: 'default',
							autoGainControl: false,
							channelCount: 2,
							echoCancellation: false,
							latency: 0,
							noiseSuppression: false,
							sampleRate: 48000,
							sampleSize: 16,
							volume: 1.0
						},
						video: false
					}).then((stream) => {
						audioSender = webRTCConn.peerConnection.addTrack(stream.getAudioTracks()[0]);
					}).catch((err) => {
						console.error(err)
					});
				}
				mediaElementContainer.appendChild(button);
				mediaElementContainer.appendChild(document.createElement("br"));
			}
		}
		mediaElementContainer.appendChild(document.createElement("br"));
		mediaElementContainer.appendChild(mediaElement);
		streamContainer.appendChild(mediaElementContainer);
	}

	for (const name of namesResp.names) {
		const container = document.createElement("div");
		container.id = `stream-${name}`;
		const button = document.createElement("button");
		button.innerText = `Start ${name}`
		button.onclick = makeButtonClick(button, name, true);
		container.appendChild(button);
		document.body.appendChild(container);
	}
}
startup().catch(e => {
	console.error(e);
});
