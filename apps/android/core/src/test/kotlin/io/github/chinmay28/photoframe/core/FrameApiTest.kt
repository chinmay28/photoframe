package io.github.chinmay28.photoframe.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class FrameApiTest {
    @Test
    fun readsTheStatus() {
        // The shape of internal/api.StatusResponse.
        val body = """{"name":"Hallway","player":{"screen":"photo","paused":true,"refreshing":false,"total":42},"upNext":[]}"""
        assertEquals(FrameApi.Status("Hallway", paused = true, total = 42), FrameApi.status(body))
        assertEquals(false, FrameApi.status("""{"name":"","player":{"screen":"welcome","total":0}}""")!!.paused)
        assertNull(FrameApi.status("""{"error":"no"}"""))
        assertNull(FrameApi.status("<html>"))
    }

    @Test
    fun picksTheUploadSourceThePageWould() {
        val sources = """[
            {"id":"nas","name":"NAS","type":"folder","enabled":true},
            {"id":"old","name":"Old uploads","type":"upload","enabled":false},
            {"id":"uploads","name":"Uploaded from phones","type":"upload","enabled":true}
        ]"""
        assertEquals(FrameApi.UploadSource("uploads", "Uploaded from phones"), FrameApi.uploadSource(sources))
        // None enabled: the first upload source still takes it, as on the page.
        assertEquals("old", FrameApi.uploadSource("""[{"id":"old","type":"upload","enabled":false}]""")!!.id)
        assertNull(FrameApi.uploadSource("""[{"id":"nas","type":"folder","enabled":true}]"""))
        assertNull(FrameApi.uploadSource("[]"))
        assertNull(FrameApi.uploadSource("""{"error":"x"}"""))
    }

    @Test
    fun readsUploadResultsAndErrors() {
        assertEquals(
            FrameApi.UploadResult(listOf("a.jpg"), listOf("b.txt: not a JPEG, PNG, GIF or WebP file")),
            FrameApi.uploadResult("""{"saved":["a.jpg"],"rejected":["b.txt: not a JPEG, PNG, GIF or WebP file"]}"""),
        )
        assertEquals(FrameApi.UploadResult(emptyList(), emptyList()), FrameApi.uploadResult("""{"saved":[]}"""))
        assertEquals("no such source", FrameApi.error("""{"error":"no such source"}"""))
        assertNull(FrameApi.error("""{"saved":[]}"""))
        assertNull(FrameApi.error("gateway timeout"))
    }

    @Test
    fun pathsAreTheServersRoutes() {
        assertEquals("/api/control/next", FrameApi.control("next"))
        assertEquals("/api/sources/uploads/photos", FrameApi.upload("uploads"))
        assertEquals("/api/sources/a%2Fb%20c/photos", FrameApi.upload("a/b c"))
    }

    @Test
    fun controlsSendTheFramesCommands() {
        assertEquals("next", Control.NEXT.command(paused = false))
        assertEquals("prev", Control.PREVIOUS.command(paused = true))
        assertEquals("pause", Control.PAUSE_OR_RESUME.command(paused = false))
        assertEquals("resume", Control.PAUSE_OR_RESUME.command(paused = true))
        assertTrue(Control.PAUSE_OR_RESUME.needsStatus)
        assertFalse(Control.NEXT.needsStatus)
        assertEquals(Control.NEXT, Control.of(Control.NEXT.action))
        assertNull(Control.of("android.intent.action.MAIN"))
        assertNull(Control.of(null))
    }

    // The launcher shortcuts are static XML, so their actions are spelled
    // out there by hand; each must be one Control reads, and each Control
    // must have a shortcut.
    @Test
    fun everyShortcutIsAControlTheAppKnows() {
        val xml = File("../app/src/main/res/xml/shortcuts.xml").readText()
        val actions = Regex("""android:action="([^"]+)"""").findAll(xml).map { it.groupValues[1] }.toList()
        for (c in Control.entries) assertTrue("no shortcut for ${c.action}", c.action in actions)
        for (a in actions.filter { it.contains(".action.") }) assertTrue("shortcut $a is no Control", Control.of(a) != null)
    }
}
