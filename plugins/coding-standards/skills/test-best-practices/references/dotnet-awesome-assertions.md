# .NET Assertions: FluentAssertions / AwesomeAssertions + NUnit

AwesomeAssertions is a fork of FluentAssertions — the API is identical. Everything in this reference applies to both libraries. **Every example here is written in FluentAssertions**, the assumed baseline; translate the namespace if you are on AwesomeAssertions.

Technology-specific patterns implementing the principles from the parent skill.

## Pick the Analyzer Package That Matches Your Assertion Library

| Assertion library | Analyzer package |
|---|---|
| FluentAssertions 6.x | `FluentAssertions.Analyzers` |
| AwesomeAssertions 9.x | `AwesomeAssertions.Analyzers` |

**Mixing them fails silently.** `FluentAssertions.Analyzers` gates on
`Compilation.GetTypeByMetadataName("FluentAssertions.AssertionExtensions")`, which AwesomeAssertions v9
renamed away — so against AwesomeAssertions it emits **zero diagnostics and no error**. You get a
green build that has analysed nothing. Diagnostic ids and `.editorconfig` keys are identical either
way, so the only visible symptom is the absence of warnings.

## Combining Assertions: `BeEquivalentTo` with Anonymous Objects

Enforced by the custom `combine-assertions-on-same-object` analyzer; `FAA0001`–`FAA0004` steer the
count-and-index shapes.

Use `BeEquivalentTo` with an anonymous object to verify multiple properties in a single assertion. Use `ExcludingMissingMembers()` when the anonymous object is a subset of the actual.

```csharp
// BAD — back-to-back assertions on same value
result.Page.Should().Be(2);
result.PageSize.Should().Be(3);
result.TotalResults.Should().Be(10);

// GOOD — single structural assertion
result.Should().BeEquivalentTo(
    new { Page = 2, PageSize = 3, TotalResults = 10 },
    o => o.ExcludingMissingMembers());
```

### Collections

```csharp
// BAD — count then index
items.Should().HaveCount(2);
items[0].Name.Should().Be("Alice");
items[1].Name.Should().Be("Bob");

// GOOD — single equivalence
items.Should().BeEquivalentTo(new[]
{
    new { Name = "Alice" },
    new { Name = "Bob" }
}, o => o.ExcludingMissingMembers());
```

When verifying only count, a direct assertion is fine:

```csharp
body.Result.Items.Should().HaveCount(2);
```

## Communicating Meaning: Expressive Assertion Methods

Enforced off the shelf by `FluentAssertions.Analyzers` `FAA0001`–`FAA0004`.

Choose assertion methods whose names describe the expectation:

```csharp
// BAD — generic
result.Should().NotBeNull();
result.Name.Should().Be("expected");

// GOOD — assertion method tells you what's expected
result.Should().BeEquivalentTo(
    new { Name = "expected" },
    o => o.ExcludingMissingMembers());

// GOOD — expressive chain for single collection item
items.Should().ContainSingle()
    .Which.Should().BeOfType<MyType>()
    .Which.Should().BeEquivalentTo(new { Name = "expected" });
```

### Type + Value Checking — **[review-only]**

No analyzer knows you *intended* a type check, so nothing detects the omission.

`BeEquivalentTo` is purely structural — it does NOT check types. When you need to verify the object is a specific type AND has expected values:

```csharp
// BAD — only checks property values, not type
actual.Should().BeEquivalentTo(new { Name = "foo" });

// GOOD — verifies type, then checks property values
actual.Should().BeOfType<ExpectedType>()
    .Which.Should().BeEquivalentTo(new { Name = "foo" });
```

The `BeOfType<T>().Which` chain is a single fluent expression, not back-to-back assertions.

> **Strict typing note**: `WithStrictTyping()` / `WithStrictTypingFor()` are not available in
> FluentAssertions 6.x (nor in AwesomeAssertions 9.4). Use `BeOfType<T>().Which` for type enforcement.

### Why Anonymous Objects for `BeEquivalentTo`? — **[review-only]**

Heuristic and context-dependent — a typed expected object is often the right call, so this stays a
judgment rather than a rule.

Typed expected objects (e.g., `new MirthChannelAuditTrail(...)`) inherit base class properties like `Timestamp` that differ between expected and actual instances — causing spurious failures. `BeOfType<T>().Which` handles type enforcement, then anonymous objects let you check only the properties you care about.

## Scoping: `AssertionScope` — **[review-only]**

When multiple assertions on the same value truly cannot be combined into `BeEquivalentTo` (e.g., mixing type checks with property checks, or assertions with different comparison strategies), wrap them in an `AssertionScope`:

```csharp
using (new AssertionScope())
{
    response.StatusCode.Should().Be(HttpStatusCode.Created);
    response.Headers.Location.Should().Be(
        new Uri(Client.BaseAddress!, Route($"/{created.Id}")));
    body.Result.ChannelName.Should().Be(request.ChannelName);
}
```

Without the scope, only the first failure is reported and subsequent assertions are never evaluated.

## Null Safety in Assertions — **[custom rule]**

Enforced by the custom `no-suppression-before-assertion` Roslyn analyzer.

Never use `!` (null-forgiving) to dereference a potentially null value before asserting. Never use `?.` — it silently skips the assertion chain if the value is null, causing a false pass.

```csharp
// BAD — nullable dereference, poor failure message
response.Headers.Location!.OriginalString.Should().Contain("/items/123");

// BAD — silently passes if Location is null
response.Headers.Location?.OriginalString.Should().Contain("/items/123");

// GOOD — construct expected value, single assertion
response.Headers.Location.Should().Be(
    new Uri(Client.BaseAddress!, Route($"/{item.Id}")));
```

Note: `Client.BaseAddress!` is acceptable because `BaseAddress` is set during test setup and is a precondition, not the value under test. The analyzer scopes to the receiver chain feeding `.Should()` for exactly this reason.

## Audit Trail Pattern — **[review-only]**

A worked composite of the expressive-chain and type-check guidelines above, not a separate rule.

For audit trail verification, combine `ContainSingle`, `BeOfType`, and `BeEquivalentTo`:

```csharp
_auditor.AuditTrailLogs.Should().ContainSingle()
    .Which.Should().BeOfType<MirthChannelAuditTrail>()
    .Which.Should().BeEquivalentTo(new
    {
        Action = AuditExtensions.ActionMirthChannelRetrieved,
        ChannelId = channelId,
        Succeeded = true
    });
```

## Quick Reference

| Scenario | Pattern |
|----------|---------|
| Multiple properties | `BeEquivalentTo(new { ... }, o => o.ExcludingMissingMembers())` |
| Collection contents | `BeEquivalentTo(new[] { new { ... }, ... })` |
| Collection count only | `HaveCount(n)` |
| Single item + type | `ContainSingle().Which.Should().BeOfType<T>().Which.Should().BeEquivalentTo(new { ... })` |
| Type check only | `BeOfType<T>()` |
| Nullable value | Construct expected value, use `Be()` — no `!` or `?.` |
| Multiple unrelated assertions | Wrap in `AssertionScope` |
| Analyzer package | FluentAssertions 6.x → `FluentAssertions.Analyzers`; AwesomeAssertions 9.x → `AwesomeAssertions.Analyzers` |
