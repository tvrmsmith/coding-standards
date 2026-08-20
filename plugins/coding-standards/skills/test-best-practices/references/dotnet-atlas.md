# .NET Assertions: Escaping a Framework's Custom Assertions Type

Read `dotnet-awesome-assertions.md` first for general .NET assertion guidance.

## The Problem

Some frameworks ship their own `Should()` extension for their wrapper types, returning a bespoke
assertions type instead of the standard `ObjectAssertions`. Those bespoke types typically lack
`BeEquivalentTo`, so the combine-assertions guidance appears unreachable.

**Worked example — Atlas.** Atlas provides custom `Should()` extensions for `ApiResponse<T>` that
return `ApiResponseAssertions<T>`, which has no `BeEquivalentTo`.

## Solution: Assert on the Inner DTO — **[review-only]**

Choosing the right assertion target is judgment, so this half is not lintable.

Assert on the inner DTO (`body.Result`) rather than on the wrapper (`ApiOkResponse<T>`). Check the
transport-level concern — HTTP status — separately on `response.StatusCode`.

```csharp
// GOOD — assert status and result separately, no cast needed
using (new AssertionScope())
{
    response.StatusCode.Should().Be(HttpStatusCode.OK);
    body.Result.Should().BeEquivalentTo(
        new { Id = channelId },
        o => o.ExcludingMissingMembers());
}
```

Use `AssertionScope` when checking both status and body, since these are genuinely separate
assertions that can't be combined — see the scoping guidance in the parent skill (**[review-only]**).

## Never `(object)`-Cast to Escape the Custom Type — **[custom rule]**

Enforced by the custom `no-assertion-escape-cast` Roslyn analyzer, which flags a cast to `object`
whose sole purpose is to reach the general `ObjectAssertions` overload.

```csharp
// BAD — (object) cast purely to escape the framework's custom assertions type
((object)body).Should().BeEquivalentTo(
    new { StatusCode = 200, Result = new { Id = channelId } },
    o => o.ExcludingMissingMembers());
```

The cast throws away the type the framework deliberately gave you and produces failure messages
about an `object`. Assert on the inner DTO instead.
