// agent-desk-dictate turns speech from the microphone into text, on this
// Mac only: Apple's speech recognizer with on-device recognition required,
// so no audio leaves the machine. The desktop app starts it when the mic
// button is pressed and reads one JSON object per line from its stdout:
//
//   {"type":"ready"}                       listening
//   {"type":"text","text":"…","final":false}  everything heard so far
//   {"type":"error","message":"…"}         why it stopped
//   {"type":"done"}                        stopped
//
// It stops when stdin closes or receives a line, and after a long silence.
import AVFoundation
import Foundation
import Speech

setvbuf(stdout, nil, _IOLBF, 0)

func emit(_ obj: [String: Any]) {
    if let data = try? JSONSerialization.data(withJSONObject: obj),
       let line = String(data: data, encoding: .utf8) {
        print(line)
    }
}

func fail(_ message: String) -> Never {
    emit(["type": "error", "message": message])
    emit(["type": "done"])
    exit(1)
}

let engine = AVAudioEngine()
var task: SFSpeechRecognitionTask?
var request: SFSpeechAudioBufferRecognitionRequest?
var stopping = false

// On-device recognition may start its transcription afresh after a pause,
// dropping what came before. kept holds those earlier words, so the text
// sent is always everything said since the button was pressed.
var kept = ""
var last = ""

func joined(_ a: String, _ b: String) -> String {
    if a.isEmpty { return b }
    if b.isEmpty { return a }
    return a + " " + b
}

// heard returns the whole text so far for a new transcription.
func heard(_ text: String) -> String {
    // A transcription that is suddenly much shorter and not a start of the
    // last one began afresh; a revision keeps about the same length.
    if last.count >= 8 && text.count * 2 < last.count && !last.hasPrefix(text) {
        kept = joined(kept, last)
    }
    last = text
    return joined(kept, text)
}

func finish() {
    if stopping { return }
    stopping = true
    engine.stop()
    engine.inputNode.removeTap(onBus: 0)
    request?.endAudio()
    // The recognizer sends its final result shortly after the audio ends.
    DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
        task?.cancel()
        emit(["type": "done"])
        exit(0)
    }
}

// A line on stdin, or stdin closing, asks to stop.
DispatchQueue.global().async {
    _ = readLine()
    DispatchQueue.main.async { finish() }
}

// silenceLimit stops listening after this long with no new words.
let silenceLimit: TimeInterval = 30
var lastHeard = Date()
Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { _ in
    if !stopping && Date().timeIntervalSince(lastHeard) > silenceLimit { finish() }
}

func start() {
    let locale = Locale.current
    guard let recognizer = SFSpeechRecognizer(locale: locale) ?? SFSpeechRecognizer(locale: Locale(identifier: "en-US")) else {
        fail("No speech recognizer is available for \(locale.identifier).")
    }
    guard recognizer.supportsOnDeviceRecognition else {
        fail("On-device dictation is not available for \(recognizer.locale.identifier). Download it in System Settings › Keyboard › Dictation.")
    }
    guard recognizer.isAvailable else {
        fail("The speech recognizer is not available right now.")
    }
    let req = SFSpeechAudioBufferRecognitionRequest()
    req.requiresOnDeviceRecognition = true
    req.shouldReportPartialResults = true
    req.addsPunctuation = true
    request = req

    let input = engine.inputNode
    let format = input.outputFormat(forBus: 0)
    guard format.channelCount > 0 else { fail("No microphone is available.") }
    input.installTap(onBus: 0, bufferSize: 1024, format: format) { buffer, _ in
        req.append(buffer)
    }
    engine.prepare()
    do {
        try engine.start()
    } catch {
        fail("The microphone could not start: \(error.localizedDescription)")
    }
    task = recognizer.recognitionTask(with: req) { result, error in
        // Results arrive on the recognizer's queue; state lives on main.
        let text = result.map { $0.bestTranscription.formattedString }
        let isFinal = result?.isFinal ?? false
        let code = (error as NSError?)?.code
        let message = error?.localizedDescription
        DispatchQueue.main.async {
            if let text = text {
                lastHeard = Date()
                emit(["type": "text", "text": heard(text), "final": isFinal])
                if isFinal { finish() }
            }
            if let code = code, !stopping {
                // 1110 is "no speech detected", which ends a silent session.
                if code == 1110 { finish() } else { fail(message ?? "Dictation stopped.") }
            }
        }
    }
    lastHeard = Date()
    emit(["type": "ready"])
}

func requestMicrophone() {
    switch AVCaptureDevice.authorizationStatus(for: .audio) {
    case .authorized:
        start()
    case .notDetermined:
        AVCaptureDevice.requestAccess(for: .audio) { ok in
            DispatchQueue.main.async {
                if ok { start() } else { fail("Microphone access was not allowed. Turn it on for Agent Desk in System Settings › Privacy & Security › Microphone.") }
            }
        }
    default:
        fail("Microphone access is off for Agent Desk. Turn it on in System Settings › Privacy & Security › Microphone.")
    }
}

SFSpeechRecognizer.requestAuthorization { status in
    DispatchQueue.main.async {
        switch status {
        case .authorized:
            requestMicrophone()
        case .denied, .restricted:
            fail("Speech recognition is off for Agent Desk. Turn it on in System Settings › Privacy & Security › Speech Recognition.")
        default:
            fail("Speech recognition was not allowed.")
        }
    }
}

RunLoop.main.run()
