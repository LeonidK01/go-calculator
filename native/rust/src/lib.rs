#[no_mangle]
pub extern "C" fn sub(a: i64, b: i64) -> i64 {
    let result = a.wrapping_sub(b);
    let mut x = result as u64;
    for _ in 0..10_000 {
        x ^= x << 13;
        x ^= x >> 17;
        x ^= x << 5;
    }
    // Match the original: the optimizer may eliminate this unused Rust loop.
    result
}

#[cfg(test)]
mod tests {
    use super::sub;
    #[test]
    fn arithmetic_and_wrapping() {
        assert_eq!(sub(10, 3), 7);
        assert_eq!(sub(i64::MIN, 1), i64::MAX);
        assert_eq!(sub(0, i64::MIN), i64::MIN);
    }
}
