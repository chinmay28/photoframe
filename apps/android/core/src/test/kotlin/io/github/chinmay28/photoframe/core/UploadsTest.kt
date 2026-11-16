package io.github.chinmay28.photoframe.core

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.io.File

class UploadsTest {
    @Test
    fun whatTheFrameDecodesGoesAsItIs() {
        assertEquals(UploadAs.AS_IS, UploadPlan.decide("IMG_1.jpg", "image/jpeg"))
        assertEquals(UploadAs.AS_IS, UploadPlan.decide("x", "image/png"))
        assertEquals(UploadAs.AS_IS, UploadPlan.decide("a.gif", "IMAGE/GIF"))
        assertEquals(UploadAs.AS_IS, UploadPlan.decide("a.webp", "image/webp"))
        // No useful type: the extension decides.
        assertEquals(UploadAs.AS_IS, UploadPlan.decide("a.JPEG", null))
        assertEquals(UploadAs.AS_IS, UploadPlan.decide("a.png", "application/octet-stream"))
    }

    @Test
    fun otherImagesBecomeJpegsAndTheRestIsRefused() {
        assertEquals(UploadAs.JPEG, UploadPlan.decide("IMG_2.HEIC", "image/heic"))
        assertEquals(UploadAs.JPEG, UploadPlan.decide("x", "image/heif"))
        assertEquals(UploadAs.JPEG, UploadPlan.decide("x.avif", "image/avif"))
        assertEquals(UploadAs.JPEG, UploadPlan.decide("IMG_3.heic", ""))
        assertEquals(UploadAs.REFUSE, UploadPlan.decide("clip.mp4", "video/mp4"))
        assertEquals(UploadAs.REFUSE, UploadPlan.decide("notes.txt", null))
        assertEquals(UploadAs.REFUSE, UploadPlan.decide(null, null))
    }

    @Test
    fun theNameEndsInWhatIsSent() {
        assertEquals("IMG_1.jpg", UploadPlan.nameFor("IMG_1.jpg", "image/jpeg", UploadAs.AS_IS))
        assertEquals("IMG_1.JPEG", UploadPlan.nameFor("IMG_1.JPEG", "image/jpeg", UploadAs.AS_IS))
        assertEquals("IMG_2.jpg", UploadPlan.nameFor("IMG_2.HEIC", "image/heic", UploadAs.JPEG))
        assertEquals("PXL_2026.png", UploadPlan.nameFor("PXL_2026", "image/png", UploadAs.AS_IS))
        // A name that lies about its type gets the type's extension.
        assertEquals("screen.png", UploadPlan.nameFor("screen.jpg", "image/png", UploadAs.AS_IS))
        assertEquals("passwd.webp", UploadPlan.nameFor("../../passwd", "image/webp", UploadAs.AS_IS))
        assertEquals("My photo.v2.jpg", UploadPlan.nameFor("My photo.v2", "image/jpeg", UploadAs.AS_IS))
    }

    @Test
    fun namelessSharesAreNumbered() {
        assertEquals("shared-photo-1.jpg", UploadPlan.nameFor(null, "image/jpeg", UploadAs.AS_IS, 0))
        assertEquals("shared-photo-3.jpg", UploadPlan.nameFor("image:1234", "image/heic", UploadAs.JPEG, 2))
        assertEquals("shared-photo-2.png", UploadPlan.nameFor(".png", "image/png", UploadAs.AS_IS, 1))
    }

    @Test
    fun mimeFollowsThePlan() {
        assertEquals("image/jpeg", UploadPlan.mimeFor("a.heic", "image/heic", UploadAs.JPEG))
        assertEquals("image/png", UploadPlan.mimeFor("a.png", null, UploadAs.AS_IS))
        assertEquals("image/gif", UploadPlan.mimeFor("a", "image/gif; charset=x", UploadAs.AS_IS))
    }

    @Test
    fun conversionsAreCappedButNeverEnlarged() {
        assertEquals(4096 to 3072, UploadPlan.convertedSize(8064, 6048))
        assertEquals(3072 to 4096, UploadPlan.convertedSize(6048, 8064))
        assertEquals(4032 to 3024, UploadPlan.convertedSize(4032, 3024))
        assertEquals(0 to 0, UploadPlan.convertedSize(0, 0))
    }

    @Test
    fun quotesNamesTheWayBrowsersDo() {
        assertEquals("say %22cheese%22.jpg", Multipart.quote("say \"cheese\".jpg"))
        assertEquals("a%0D%0Ab.jpg", Multipart.quote("a\r\nb.jpg"))
        assertEquals("Café.jpg", Multipart.quote("Café.jpg"))
    }

    @Test
    fun boundariesAreValid() {
        val b = Multipart.randomBoundary()
        assertTrue(b, Multipart(b).contentType.endsWith("boundary=$b"))
        assertTrue(Multipart.randomBoundary() != b)
        assertTrue(runCatching { Multipart("has space") }.isFailure)
        assertTrue(runCatching { Multipart("") }.isFailure)
    }

    // The body the share sheet sends, byte for byte. The frame's own test
    // (internal/api/android_test.go) posts this same file to the real upload
    // handler, so if either side changes, one of the two tests fails.
    @Test
    fun writesTheBodyTheFrameIsTestedWith() {
        val testdata = File("../../../internal/api/testdata")
        val mp = Multipart("PhotoframeAndroidTestBoundary")
        val out = ByteArrayOutputStream()
        File(testdata, "android-upload/dot.png").inputStream().use { mp.writeFile(out, "photos", "Café dot.png", "image/png", it) }
        File(testdata, "android-upload/dot.jpg").inputStream().use { mp.writeFile(out, "photos", "say \"cheese\".jpg", "image/jpeg", it) }
        mp.finish(out)
        assertArrayEquals(File(testdata, "android-upload.multipart").readBytes(), out.toByteArray())
    }
}
