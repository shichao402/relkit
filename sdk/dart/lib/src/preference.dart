/// Rank mirrors from real download history only (SPEC.md section 12.7).
///
/// No probe / speed-test traffic: callers may only call [UpdateState.recordSourceSuccess]
/// / [UpdateState.recordSourceFailure] after authentic directory, index, manifest, or
/// artifact attempts.
library;

import 'state.dart';

/// Stable reorder: last success → higher historical bytes/sec → original order.
///
/// [items] is the default sequence (already priority-sorted for directory services).
List<T> rankByLearning<T>({
  required List<T> items,
  required String Function(T item) keyOf,
  required UpdateState state,
}) {
  if (items.length <= 1) return List<T>.of(items);

  final indexed = <({T item, int index, String key})>[
    for (var i = 0; i < items.length; i++)
      (item: items[i], index: i, key: keyOf(items[i])),
  ];

  indexed.sort((a, b) {
    final aLast = state.lastSuccessfulSourceKey == a.key;
    final bLast = state.lastSuccessfulSourceKey == b.key;
    if (aLast != bLast) return aLast ? -1 : 1;

    final aBps = state.sourceStats[a.key]?.lastBytesPerSecond ?? -1;
    final bBps = state.sourceStats[b.key]?.lastBytesPerSecond ?? -1;
    if (aBps != bBps) return bBps.compareTo(aBps);

    final aFail = state.sourceStats[a.key]?.consecutiveFailures ?? 0;
    final bFail = state.sourceStats[b.key]?.consecutiveFailures ?? 0;
    if (aFail != bFail) return aFail.compareTo(bFail);

    return a.index.compareTo(b.index);
  });

  return [for (final row in indexed) row.item];
}

List<Uri> rankUris(List<Uri> urls, UpdateState state) => rankByLearning<Uri>(
      items: urls,
      keyOf: (uri) => uri.toString(),
      state: state,
    );

List<String> rankUrlStrings(List<String> urls, UpdateState state) =>
    rankByLearning<String>(
      items: urls,
      keyOf: (url) => url,
      state: state,
    );
