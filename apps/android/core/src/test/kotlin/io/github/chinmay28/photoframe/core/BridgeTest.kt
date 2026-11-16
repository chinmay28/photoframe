package io.github.chinmay28.photoframe.core

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class BridgeTest {
    @Test
    fun readsTheTwoMessages() {
        assertEquals(BridgeRequest.Hello(1), BridgeRequest.parse("""{"t":"hello","re":1}""").getOrThrow())
        assertEquals(BridgeRequest.OpenSettings(7), BridgeRequest.parse("""{"t":"settings","re":7}""").getOrThrow())
    }

    @Test
    fun refusesAnythingElseButCanStillAnswerIt() {
        for (bad in listOf("""{"t":"begin","re":3}""", """{"re":3}""", "not json")) {
            assertTrue(bad, BridgeRequest.parse(bad).isFailure)
        }
        assertEquals(3, BridgeRequest.reOf("""{"t":"begin","re":3}"""))
        assertEquals(0, BridgeRequest.reOf("not json"))
    }

    @Test
    fun answersCarryTheRequestsNumber() {
        val ok = JSONObject(BridgeReply.ok(4, "platform" to "android"))
        assertEquals(4, ok.getInt("re"))
        assertTrue(ok.getBoolean("ok"))
        assertEquals("android", ok.getString("platform"))
        val err = JSONObject(BridgeReply.error(5, "no"))
        assertEquals(false, err.getBoolean("ok"))
        assertEquals("no", err.getString("error"))
    }

    // The page side: assets/photoframe-bridge.js sends only what parse reads,
    // under the object name PageBridge listens on.
    @Test
    fun theScriptSpeaksThisProtocol() {
        val js = File("../app/src/main/assets/photoframe-bridge.js").readText()
        assertTrue(js.contains("window.PhotoframeAndroid"))
        assertTrue(js.contains("window.photoframeApp"))
        val sent = Regex("""t: '([a-z]+)'""").findAll(js).map { it.groupValues[1] }.toSet()
        assertEquals(setOf("hello", "settings"), sent)
        for (t in sent) assertTrue(t, BridgeRequest.parse("""{"t":"$t","re":1}""").isSuccess)
    }
}
