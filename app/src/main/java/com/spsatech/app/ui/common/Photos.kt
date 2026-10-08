package com.spsatech.app.ui.common

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Matrix
import android.net.Uri
import androidx.core.content.FileProvider
import androidx.exifinterface.media.ExifInterface
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.ByteArrayOutputStream
import java.io.File
import kotlin.math.max

private const val MAX_SIDE = 1600
private const val JPEG_QUALITY = 85

/**
 * Готовит фото к отправке: уменьшает до 1600 px по большей стороне,
 * поворачивает по EXIF и пережимает в JPEG. Так любое фото (в т.ч. HEIC)
 * уходит на сервер в поддерживаемом формате и весит ~200–500 КБ.
 */
suspend fun compressPhoto(context: Context, uri: Uri): ByteArray = withContext(Dispatchers.IO) {
    val resolver = context.contentResolver
    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    resolver.openInputStream(uri).use { BitmapFactory.decodeStream(it, null, bounds) }
    require(bounds.outWidth > 0 && bounds.outHeight > 0) { "Не удалось прочитать фото" }

    var sample = 1
    while (max(bounds.outWidth, bounds.outHeight) / (sample * 2) >= MAX_SIDE) sample *= 2
    val decoded = resolver.openInputStream(uri).use {
        BitmapFactory.decodeStream(it, null, BitmapFactory.Options().apply { inSampleSize = sample })
    } ?: error("Не удалось прочитать фото")

    val rotation = resolver.openInputStream(uri).use { stream ->
        stream?.let { ExifInterface(it).rotationDegrees } ?: 0
    }
    val scale = MAX_SIDE.toFloat() / max(decoded.width, decoded.height)
    val matrix = Matrix().apply {
        if (scale < 1f) postScale(scale, scale)
        if (rotation != 0) postRotate(rotation.toFloat())
    }
    val result = if (matrix.isIdentity) decoded
    else Bitmap.createBitmap(decoded, 0, 0, decoded.width, decoded.height, matrix, true)

    ByteArrayOutputStream().use { out ->
        result.compress(Bitmap.CompressFormat.JPEG, JPEG_QUALITY, out)
        if (result !== decoded) result.recycle()
        decoded.recycle()
        out.toByteArray()
    }
}

/** Временный файл для снимка с камеры (см. res/xml/file_paths.xml). */
fun newCameraUri(context: Context): Uri {
    val dir = File(context.cacheDir, "camera").apply { mkdirs() }
    val file = File.createTempFile("photo_", ".jpg", dir)
    return FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
}

