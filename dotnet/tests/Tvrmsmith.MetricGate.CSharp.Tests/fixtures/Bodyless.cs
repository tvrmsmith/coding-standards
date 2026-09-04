using System;

namespace Fixtures;

// A declaration carrying no body carries no lines to score, so it gets no span. Every way C# has
// of writing one is here, methods and auto-properties alike, so a guard that stops excluding one
// of them fails on its own name, down to the bodyless local function at the bottom, whose only
// span is the method holding it.
public interface IThing
{
    int Id { get; set; }

    int Compute(int a);
}

public abstract class Thing
{
    public abstract int Compute(int a);

    public abstract int Ordinal { get; set; }

    public event EventHandler? Changed;

    public extern int Extern(int a);

    public extern int Native { get; set; }
}

public partial class Halves
{
    partial void OnDone();
}

public record Order(int Id);

public class Primary(int seed)
{
    private readonly int _seed = seed;
}

public class Holder
{
    public void Hold()
    {
        static extern void Promised(int a);
    }
}
