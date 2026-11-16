package io.github.chinmay28.photoframe.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppConfigTest {
    @Test
    fun nothingUntilAFrameIsChosen() {
        assertFalse(AppConfig().configured)
        assertTrue(AppConfig().origin().isFailure)
        assertFalse(AppConfig("  ").configured)
    }

    @Test
    fun theAddressIsReadAsTyped() {
        assertEquals("http://photoframe.local", AppConfig("photoframe.local").origin().getOrThrow().toString())
        assertTrue(AppConfig("ftp://x").origin().isFailure)
    }

    @Test
    fun sharesAreReadOnlyFromContentProviders() {
        val got = ShareInput.accept(
            listOf(
                "content://media/external/images/media/1",
                "file:///data/data/io.github.chinmay28.photoframe/shared_prefs/photoframe.xml",
                "content://media/external/images/media/1",
                "CONTENT://com.google.android.apps.photos.contentprovider/0/1",
                "http://evil.test/x.jpg",
            ),
        )
        assertEquals(listOf("content://media/external/images/media/1", "CONTENT://com.google.android.apps.photos.contentprovider/0/1"), got)
        assertEquals(ShareInput.MAX_FILES, ShareInput.accept(List(500) { "content://m/$it" }).size)
    }
}
