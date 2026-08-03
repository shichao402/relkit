/// Errors raised while staging or applying an update package.
class ApplyException implements Exception {
  ApplyException(this.message, {this.cause});

  final String message;
  final Object? cause;

  @override
  String toString() =>
      'ApplyException: $message${cause == null ? '' : '\n  caused by: $cause'}';
}
