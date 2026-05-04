// Package imagehash реализует perceptual image matching для статичного медиа.
//
// Matcher скачивает контент, извлекает первый кадр и считает goimagehash
// PerceptionHash. pHash используется как MVP image fingerprint, потому что он
// достаточно устойчив к resize, recompression и легким изменениям цвета.
//
// Для каждого изображения сохраняются pHash варианты для 0, 90, 180 и 270
// градусов. Hash values хранятся как uint64, а поиск идет линейным индексом
// через Hamming distance:
//
//	bits.OnesCount64(query ^ stored) <= threshold
//
// Такой индекс прост, не аллоцирует на hot path и подходит как baseline для
// blacklist порядка десятков тысяч записей. Если blacklist вырастет или
// появится высокая нагрузка, индекс можно заменить на BK-tree, MIH или другой
// exact radius search по Hamming space, не меняя логику Matcher.
//
// NewSQLiteMatcher использует SQLite как persistent source of truth. При
// старте он загружает сохраненные hashes в in-memory LinearIndex; при Block
// сначала пишет запись в SQLite, затем добавляет ее в индекс. Записи
// дедуплицируются по hash version и deterministic signature набора variants,
// чтобы повторный ban той же картинки не раздувал базу и индекс.
//
// Этот matcher не является semantic или object detector. Сильный crop,
// overlays, collage edits, однотонные изображения, meme edits и тяжелые redraws
// все еще могут давать false negatives или false positives. Такие случаи нужно
// закрывать следующими слоями модерации: region hashes, PDQ, local features или
// embeddings после калибровки на реальных данных.
package imagehash
